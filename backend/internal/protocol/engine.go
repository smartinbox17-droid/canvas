package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const MaxEvalDepth = 32

type MediaReference struct {
	ID       string  `json:"id,omitempty"`
	URL      string  `json:"url,omitempty"`
	DataURL  string  `json:"dataUrl,omitempty"`
	Kind     string  `json:"kind,omitempty"`     // image | video | audio
	Role     string  `json:"role,omitempty"`     // first_frame | last_frame | reference | mask
	MimeType string  `json:"mimeType,omitempty"`
	Order    int     `json:"order,omitempty"`
	Weight   float64 `json:"weight,omitempty"`
}

type OutputOptions struct {
	Count         int    `json:"count,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	AspectRatio   string `json:"aspectRatio,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
	Quality       string `json:"quality,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	GenerateAudio bool   `json:"generateAudio,omitempty"`
	Format        string `json:"format,omitempty"`
}

type GenerationRequest struct {
	Capability      string                 `json:"capability"` // text | image | video | audio
	Model           string                 `json:"model"`
	Prompt          string                 `json:"prompt"`
	NegativePrompt  string                 `json:"negativePrompt,omitempty"`
	Images          []MediaReference       `json:"images,omitempty"`
	Videos          []MediaReference       `json:"videos,omitempty"`
	Audios          []MediaReference       `json:"audios,omitempty"`
	Output          OutputOptions          `json:"output,omitempty"`
	ProviderOptions map[string]any         `json:"providerOptions,omitempty"`
	Extra           map[string]any         `json:"extra,omitempty"`
}

type Manifest struct {
	APIVersion    string            `json:"apiVersion"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Author        string            `json:"author,omitempty"`
	Description   string            `json:"description,omitempty"`
	Permissions   []string          `json:"permissions,omitempty"`
	Configuration ManifestConfig    `json:"configuration,omitempty"`
	Contributes   ManifestContrib   `json:"contributes"`
}

type ManifestConfig struct {
	Fields []ConfigField `json:"fields,omitempty"`
}

type ConfigField struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // secret | string | number | boolean
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type ManifestContrib struct {
	Providers []ProviderSpec `json:"providers"`
}

type ProviderSpec struct {
	ID                      string              `json:"id"`
	Label                   string              `json:"label"`
	Capabilities            []string            `json:"capabilities"`
	Scopes                  []string            `json:"scopes,omitempty"`
	BaseURL                 string              `json:"baseUrl"`
	RequiresPublicMediaURLs bool                `json:"requiresPublicMediaUrls,omitempty"`
	Auth                    AuthSpec            `json:"auth"`
	Request                 ProviderRequestSpec `json:"request"`
	Response                ResponseSpec        `json:"response"`
}

type AuthSpec struct {
	Type   string `json:"type"`             // bearer | header | query | none
	Field  string `json:"field,omitempty"`  // e.g. "apiKey"
	Header string `json:"header,omitempty"` // e.g. "X-API-Key"
	Prefix string `json:"prefix,omitempty"`
}

type ProviderRequestSpec struct {
	Create *OperationSpec `json:"create,omitempty"`
	Poll   *OperationSpec `json:"poll,omitempty"`
	Result *OperationSpec `json:"result,omitempty"`
	Cancel *OperationSpec `json:"cancel,omitempty"`
}

type OperationSpec struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	ContentType string            `json:"contentType,omitempty"`
	Headers     map[string]any    `json:"headers,omitempty"`
	Query       map[string]any    `json:"query,omitempty"`
	Body        map[string]any    `json:"body,omitempty"`
}

type ResponseSpec struct {
	TaskID          any               `json:"taskId,omitempty"`
	Status          any               `json:"status,omitempty"`
	StatusMap       map[string]string `json:"statusMap,omitempty"`
	Message         any               `json:"message,omitempty"`
	Text            any               `json:"text,omitempty"`
	Images          any               `json:"images,omitempty"`
	Videos          any               `json:"videos,omitempty"`
	Audios          any               `json:"audios,omitempty"`
	ResultEphemeral bool              `json:"resultEphemeral,omitempty"`
}

type NormalizedResponse struct {
	TaskID          string   `json:"taskId,omitempty"`
	Status          string   `json:"status"` // pending | processing | succeeded | failed | cancelled
	Message         string   `json:"message,omitempty"`
	Text            string   `json:"text,omitempty"`
	Images          []string `json:"images,omitempty"`
	Videos          []string `json:"videos,omitempty"`
	Audios          []string `json:"audios,omitempty"`
	ResultEphemeral bool     `json:"resultEphemeral,omitempty"`
	Raw             any      `json:"raw,omitempty"`
}

type omitSentinel struct{}

var omitValue = omitSentinel{}

// Eval evaluates a declarative plugin expression tree against a pure context map.
func Eval(expr any, scope map[string]any) (any, error) {
	return evalDepth(expr, scope, 0)
}

func evalDepth(expr any, scope map[string]any, depth int) (any, error) {
	if depth > MaxEvalDepth {
		return nil, errors.New("dsl evaluation exceeded maximum depth")
	}
	switch v := expr.(type) {
	case nil, bool, float64, int, int64:
		return v, nil
	case string:
		return interpolateTemplate(v, scope), nil
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			ev, err := evalDepth(item, scope, depth+1)
			if err != nil {
				return nil, err
			}
			if _, isOmit := ev.(omitSentinel); !isOmit {
				out = append(out, ev)
			}
		}
		return out, nil
	case map[string]any:
		if len(v) == 1 {
			for k, rawArg := range v {
				if strings.HasPrefix(k, "$") {
					return evalOperator(k, rawArg, scope, depth+1)
				}
			}
		}
		out := make(map[string]any, len(v))
		for k, val := range v {
			if strings.HasPrefix(k, "$") {
				return nil, fmt.Errorf("unexpected operator key %q in multi-key object", k)
			}
			ev, err := evalDepth(val, scope, depth+1)
			if err != nil {
				return nil, err
			}
			if _, isOmit := ev.(omitSentinel); !isOmit {
				out[k] = ev
			}
		}
		return out, nil
	default:
		return v, nil
	}
}

func evalOperator(op string, arg any, scope map[string]any, depth int) (any, error) {
	switch op {
	case "$ref":
		pathStr, ok := arg.(string)
		if !ok {
			return nil, errors.New("$ref requires a string path")
		}
		return lookupPath(scope, pathStr), nil

	case "$coalesce":
		arr, ok := arg.([]any)
		if !ok {
			return nil, errors.New("$coalesce requires an array")
		}
		for _, item := range arr {
			val, err := evalDepth(item, scope, depth+1)
			if err != nil {
				return nil, err
			}
			if !isEmptyValue(val) {
				return val, nil
			}
		}
		return nil, nil

	case "$omitEmpty":
		val, err := evalDepth(arg, scope, depth+1)
		if err != nil {
			return nil, err
		}
		if isEmptyValue(val) {
			return omitValue, nil
		}
		return val, nil

	case "$toString":
		val, err := evalDepth(arg, scope, depth+1)
		if err != nil {
			return nil, err
		}
		if isEmptyValue(val) {
			return "", nil
		}
		return fmt.Sprintf("%v", val), nil

	case "$len":
		val, err := evalDepth(arg, scope, depth+1)
		if err != nil {
			return nil, err
		}
		if arr, ok := toSlice(val); ok {
			return len(arr), nil
		}
		if s, ok := val.(string); ok {
			return len(s), nil
		}
		return 0, nil

	case "$eq", "$ne", "$gt", "$lt":
		arr, ok := arg.([]any)
		if !ok || len(arr) != 2 {
			return nil, fmt.Errorf("%s requires a 2-element array", op)
		}
		left, err := evalDepth(arr[0], scope, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := evalDepth(arr[1], scope, depth+1)
		if err != nil {
			return nil, err
		}
		return compareValues(op, left, right), nil

	case "$if":
		m, ok := arg.(map[string]any)
		if !ok {
			return nil, errors.New("$if requires an object with condition/then/else")
		}
		cond, err := evalDepth(m["condition"], scope, depth+1)
		if err != nil {
			return nil, err
		}
		if toBool(cond) {
			return evalDepth(m["then"], scope, depth+1)
		}
		return evalDepth(m["else"], scope, depth+1)

	case "$sortByOrder":
		val, err := evalDepth(arg, scope, depth+1)
		if err != nil {
			return nil, err
		}
		arr, ok := toSlice(val)
		if !ok {
			return []any{}, nil
		}
		cloned := make([]any, len(arr))
		copy(cloned, arr)
		sort.SliceStable(cloned, func(i, j int) bool {
			oi, _ := toFloat(lookupPath(cloned[i], "order"))
			oj, _ := toFloat(lookupPath(cloned[j], "order"))
			return oi < oj
		})
		return cloned, nil

	case "$filter":
		m, ok := arg.(map[string]any)
		if !ok {
			return nil, errors.New("$filter requires an object with from/as/where")
		}
		fromVal, err := evalDepth(m["from"], scope, depth+1)
		if err != nil {
			return nil, err
		}
		arr, ok := toSlice(fromVal)
		if !ok {
			return []any{}, nil
		}
		asVar, _ := m["as"].(string)
		if asVar == "" {
			asVar = "item"
		}
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			childScope := cloneScope(scope)
			childScope[asVar] = item
			keep, err := evalDepth(m["where"], childScope, depth+1)
			if err != nil {
				return nil, err
			}
			if toBool(keep) {
				out = append(out, item)
			}
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unknown expression operator: %s", op)
	}
}

var templateVarRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func interpolateTemplate(s string, scope map[string]any) string {
	return templateVarRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := templateVarRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		val := lookupPath(scope, sub[1])
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	})
}

func lookupPath(root any, path string) any {
	parts := strings.Split(path, ".")
	cur := root
	for _, p := range parts {
		if cur == nil {
			return nil
		}
		switch m := cur.(type) {
		case map[string]any:
			cur = m[p]
		default:
			// Struct or slice support via JSON round-trip normalization
			b, err := json.Marshal(cur)
			if err != nil {
				return nil
			}
			var generic map[string]any
			if err := json.Unmarshal(b, &generic); err != nil {
				return nil
			}
			cur = generic[p]
		}
	}
	return cur
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	if _, ok := v.(omitSentinel); ok {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func toSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if arr, ok := v.([]any); ok {
		return arr, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b != "" && b != "false" && b != "0"
	case float64:
		return b != 0
	case int:
		return b != 0
	default:
		return v != nil
	}
}

func compareValues(op string, a, b any) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		switch op {
		case "$eq":
			return af == bf
		case "$ne":
			return af != bf
		case "$gt":
			return af > bf
		case "$lt":
			return af < bf
		}
	}
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	switch op {
	case "$eq":
		return as == bs
	case "$ne":
		return as != bs
	case "$gt":
		return as > bs
	case "$lt":
		return as < bs
	default:
		return false
	}
}

func cloneScope(s map[string]any) map[string]any {
	out := make(map[string]any, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	return out
}

func RequestToScopeMap(req GenerationRequest) map[string]any {
	b, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	// Flatten common output options onto request.* as well for manifest convenience
	if req.Output.Duration > 0 {
		m["duration"] = req.Output.Duration
	}
	if req.Output.AspectRatio != "" {
		m["aspectRatio"] = req.Output.AspectRatio
	}
	if req.Output.Resolution != "" {
		m["resolution"] = req.Output.Resolution
	}
	return m
}

type UpstreamCallError struct {
	StatusCode int
	Retryable  bool
	Message    string
}

func (e *UpstreamCallError) Error() string {
	return e.Message
}

func ExecuteOperation(
	ctx context.Context,
	client *http.Client,
	provider ProviderSpec,
	baseURL string,
	opSpec *OperationSpec,
	scope map[string]any,
) (map[string]any, error) {
	if opSpec == nil {
		return nil, errors.New("operation spec is nil")
	}
	effectiveBase := strings.TrimRight(baseURL, "/")
	if effectiveBase == "" {
		effectiveBase = strings.TrimRight(provider.BaseURL, "/")
	}
	path := interpolateTemplate(opSpec.Path, scope)
	fullURL := effectiveBase + "/" + strings.TrimLeft(path, "/")

	var bodyReader io.Reader
	if len(opSpec.Body) > 0 {
		evaluatedBody, err := Eval(opSpec.Body, scope)
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(evaluatedBody)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(payload)
	}

	method := strings.ToUpper(opSpec.Method)
	if method == "" {
		method = http.MethodPost
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Apply authentication
	cfgMap, _ := scope["config"].(map[string]any)
	switch provider.Auth.Type {
	case "bearer":
		if secret, _ := cfgMap[provider.Auth.Field].(string); secret != "" {
			httpReq.Header.Set("Authorization", "Bearer "+secret)
		}
	case "header":
		if secret, _ := cfgMap[provider.Auth.Field].(string); secret != "" && provider.Auth.Header != "" {
			httpReq.Header.Set(provider.Auth.Header, provider.Auth.Prefix+secret)
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &UpstreamCallError{StatusCode: 0, Retryable: true, Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &UpstreamCallError{StatusCode: resp.StatusCode, Retryable: true, Message: err.Error()}
	}

	if resp.StatusCode >= 400 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, &UpstreamCallError{
			StatusCode: resp.StatusCode,
			Retryable:  retryable,
			Message:    fmt.Sprintf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBytes))),
		}
	}

	var decoded map[string]any
	if len(rawBytes) > 0 {
		if err := json.Unmarshal(rawBytes, &decoded); err != nil {
			decoded = map[string]any{"rawText": string(rawBytes)}
		}
	} else {
		decoded = map[string]any{}
	}
	return decoded, nil
}

func NormalizeResponse(spec ResponseSpec, rawResp map[string]any, baseScope map[string]any) (NormalizedResponse, error) {
	scope := cloneScope(baseScope)
	scope["response"] = rawResp

	taskIDVal, err := Eval(spec.TaskID, scope)
	if err != nil {
		return NormalizedResponse{}, err
	}
	statusVal, err := Eval(spec.Status, scope)
	if err != nil {
		return NormalizedResponse{}, err
	}
	msgVal, _ := Eval(spec.Message, scope)
	textVal, _ := Eval(spec.Text, scope)
	imgVal, _ := Eval(spec.Images, scope)
	vidVal, _ := Eval(spec.Videos, scope)
	audVal, _ := Eval(spec.Audios, scope)

	statusStr := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", statusVal)))
	if statusVal == nil || statusStr == "" {
		statusStr = "succeeded"
	}
	if mapped, ok := spec.StatusMap[statusStr]; ok {
		statusStr = mapped
	}

	return NormalizedResponse{
		TaskID:          stringOrEmpty(taskIDVal),
		Status:          statusStr,
		Message:         stringOrEmpty(msgVal),
		Text:            stringOrEmpty(textVal),
		Images:          toStringSlice(imgVal),
		Videos:          toStringSlice(vidVal),
		Audios:          toStringSlice(audVal),
		ResultEphemeral: spec.ResultEphemeral,
		Raw:             rawResp,
	}, nil
}

func stringOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && s != "" {
		return []string{s}
	}
	arr, ok := toSlice(v)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if s := stringOrEmpty(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}
