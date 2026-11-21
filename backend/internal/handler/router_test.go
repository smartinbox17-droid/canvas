package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartinbox17-droid/canvas/backend/internal/database"
	"github.com/smartinbox17-droid/canvas/backend/internal/handler"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/outbound"
	"github.com/smartinbox17-droid/canvas/backend/internal/payment"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"github.com/smartinbox17-droid/canvas/backend/internal/protocol"
	"github.com/smartinbox17-droid/canvas/backend/internal/service"
)

func newTestApp(t *testing.T) (*service.Application, http.Handler) {
	t.Helper()
	cfg := platform.Config{
		DatabaseDriver:        "sqlite",
		DataDir:               t.TempDir(),
		AutoMigrate:           true,
		RegistrationEnabled:   true,
		AllowPrivateUpstreams: true,
		CORSOrigins:           []string{"http://localhost:3000"},
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	coord := platform.NewMemoryCoordinator()
	app, err := service.NewApplication(cfg, db, coord)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app, handler.New(app).NewRouter()
}

func TestHealthAndEnvelopeContract(t *testing.T) {
	_, router := newTestApp(t)

	for _, path := range []string{"/api/health/live", "/api/health/startup", "/api/health/ready", "/api/v1/auth/settings", "/api/v1/auth/session"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned status %d: %s", path, rec.Code, rec.Body.String())
		}
		var env kernel.Envelope[map[string]any]
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("GET %s invalid JSON envelope: %v", path, err)
		}
		if env.Code != 0 || env.Msg != "ok" || env.Reason != "" {
			t.Fatalf("GET %s unexpected envelope: %+v", path, env)
		}
	}

	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec404.Code)
	}
	var env404 kernel.Envelope[any]
	if err := json.Unmarshal(rec404.Body.Bytes(), &env404); err != nil {
		t.Fatalf("invalid 404 envelope: %v", err)
	}
	if env404.Reason != kernel.ReasonNotFound {
		t.Fatalf("expected reason NOT_FOUND, got %s", env404.Reason)
	}
}

func TestPhase1AuthIsolationAndAssetReferenceGuard(t *testing.T) {
	app, router := newTestApp(t)

	cipherText, err := app.SecretBox.Encrypt("sk-live-secret-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plainText, err := app.SecretBox.Decrypt(cipherText)
	if err != nil || plainText != "sk-live-secret-key" {
		t.Fatalf("decrypt mismatch: %v %q", err, plainText)
	}

	cookieA := registerUser(t, router, "alice@example.com", "password123")
	cookieB := registerUser(t, router, "bob@example.com", "password123")

	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x01, 0x02, 0x03}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "hero.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = fw.Write(pngHeader)
	_ = mw.Close()

	upReq := httptest.NewRequest(http.MethodPost, "/api/v1/assets", &body)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upReq.AddCookie(cookieA)
	upRec := httptest.NewRecorder()
	router.ServeHTTP(upRec, upReq)
	if upRec.Code != http.StatusOK {
		t.Fatalf("upload failed (%d): %s", upRec.Code, upRec.Body.String())
	}
	var upEnv kernel.Envelope[map[string]any]
	_ = json.Unmarshal(upRec.Body.Bytes(), &upEnv)
	assetID, _ := upEnv.Data["id"].(string)
	if assetID == "" || upEnv.Data["kind"] != "image" {
		t.Fatalf("unexpected uploaded asset: %+v", upEnv.Data)
	}

	delByB := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+assetID, nil)
	delByB.AddCookie(cookieB)
	recByB := httptest.NewRecorder()
	router.ServeHTTP(recByB, delByB)
	if recByB.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when user B deletes user A asset, got %d: %s", recByB.Code, recByB.Body.String())
	}

	if err := app.Assets.BindReference(context.Background(), assetID, "canvas_node", "node_1"); err != nil {
		t.Fatalf("bind reference: %v", err)
	}
	delInUse := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+assetID, nil)
	delInUse.AddCookie(cookieA)
	recInUse := httptest.NewRecorder()
	router.ServeHTTP(recInUse, delInUse)
	if recInUse.Code != http.StatusConflict {
		t.Fatalf("expected 409 when asset is referenced, got %d: %s", recInUse.Code, recInUse.Body.String())
	}
	var envInUse kernel.Envelope[any]
	_ = json.Unmarshal(recInUse.Body.Bytes(), &envInUse)
	if envInUse.Reason != kernel.ReasonAssetInUse {
		t.Fatalf("expected ASSET_IN_USE reason, got %s", envInUse.Reason)
	}
}

func TestPhase2DeclarativeDSLAndTaskDispatchLifecycle(t *testing.T) {
	scope := map[string]any{
		"request": map[string]any{
			"model":       "sora-2",
			"prompt":      "cyberpunk neon city",
			"duration":    8,
			"aspectRatio": "",
			"images": []any{
				map[string]any{"id": "img2", "role": "mask", "order": 2},
				map[string]any{"id": "img1", "role": "first_frame", "order": 1},
			},
		},
	}
	expr := map[string]any{
		"model":   map[string]any{"$ref": "request.model"},
		"seconds": map[string]any{"$toString": map[string]any{"$ref": "request.duration"}},
		"size":    map[string]any{"$omitEmpty": map[string]any{"$ref": "request.aspectRatio"}},
		"refs": map[string]any{
			"$filter": map[string]any{
				"from":  map[string]any{"$sortByOrder": map[string]any{"$ref": "request.images"}},
				"as":    "media",
				"where": map[string]any{"$ne": []any{map[string]any{"$ref": "media.role"}, "mask"}},
			},
		},
	}
	evaluated, err := protocol.Eval(expr, scope)
	if err != nil {
		t.Fatalf("DSL Eval failed: %v", err)
	}
	evMap, _ := evaluated.(map[string]any)
	if evMap["model"] != "sora-2" || evMap["seconds"] != "8" {
		t.Fatalf("unexpected evaluated fields: %+v", evMap)
	}
	if _, hasSize := evMap["size"]; hasSize {
		t.Fatalf("expected $omitEmpty to remove empty size key: %+v", evMap)
	}
	refs, _ := evMap["refs"].([]any)
	if len(refs) != 1 {
		t.Fatalf("expected $filter to keep 1 non-mask image, got %d", len(refs))
	}

	app, router := newTestApp(t)
	srv := httptest.NewServer(router)
	defer srv.Close()

	manifestPath := filepath.Clean("../../../plugin-packages/mock-echo/manifest.json")
	if err := app.Generation.BootstrapMockCatalog(context.Background(), manifestPath, srv.URL+"/api/mock-upstream"); err != nil {
		t.Fatalf("bootstrap mock catalog: %v", err)
	}

	cookie := registerUser(t, router, "creator@example.com", "password123")

	submitBody, _ := json.Marshal(map[string]any{
		"creationSubmissionId": "idem-sub-001",
		"projectId":            "proj_1",
		"logicalModelId":       "mock-gpt-4o",
		"operation":            "text_generation",
		"request": map[string]any{
			"capability": "text",
			"model":      "mock-gpt-4o",
			"prompt":     "写一段赛博朋克短剧开场白",
		},
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(submitBody))
	req1.Header.Set("Content-Type", "application/json")
	req1.AddCookie(cookie)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("submit task failed (%d): %s", rec1.Code, rec1.Body.String())
	}
	var env1 kernel.Envelope[map[string]map[string]any]
	_ = json.Unmarshal(rec1.Body.Bytes(), &env1)
	taskID, _ := env1.Data["task"]["id"].(string)
	if taskID == "" || env1.Data["task"]["status"] != "queued" {
		t.Fatalf("unexpected task response: %+v", env1.Data)
	}

	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(submitBody))
	reqDup.Header.Set("Content-Type", "application/json")
	reqDup.AddCookie(cookie)
	recDup := httptest.NewRecorder()
	router.ServeHTTP(recDup, reqDup)
	var envDup kernel.Envelope[map[string]map[string]any]
	_ = json.Unmarshal(recDup.Body.Bytes(), &envDup)
	if envDup.Data["task"]["id"] != taskID {
		t.Fatalf("idempotency broken: expected %s, got %v", taskID, envDup.Data["task"]["id"])
	}

	processed, err := app.Generation.ProcessNextTask(context.Background())
	if err != nil || processed == nil {
		t.Fatalf("ProcessNextTask failed: %v", err)
	}
	if processed.Status != "succeeded" {
		t.Fatalf("expected task succeeded, got %s (error=%s)", processed.Status, processed.Error)
	}

	sseReq := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID+"/text-events", nil)
	sseReq.Header.Set("Last-Event-ID", "1")
	sseReq.AddCookie(cookie)
	sseRec := httptest.NewRecorder()
	router.ServeHTTP(sseRec, sseReq)
	if sseRec.Code != http.StatusOK || !strings.Contains(sseRec.Body.String(), "event: done") {
		t.Fatalf("unexpected SSE output (%d): %s", sseRec.Code, sseRec.Body.String())
	}
}

func TestPhase3PaymentRPCSettlementRedeemAndAdminRBAC(t *testing.T) {
	_, router := newTestApp(t)

	// First registered user is admin; second is regular user
	adminCookie := registerUser(t, router, "admin@example.com", "password123")
	userCookie := registerUser(t, router, "member@example.com", "password123")

	// 1. Regular user accessing /api/v1/admin/* must receive 403 FORBIDDEN
	forbidReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/overview", nil)
	forbidReq.AddCookie(userCookie)
	forbidRec := httptest.NewRecorder()
	router.ServeHTTP(forbidRec, forbidReq)
	if forbidRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", forbidRec.Code)
	}

	// 2. Admin generates a redeem batch (2 codes, 50 credits each)
	batchBody, _ := json.Marshal(map[string]any{
		"name":               "Launch Promo",
		"creditMicrocredits": int64(50_000_000),
		"count":              2,
	})
	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-batches", bytes.NewReader(batchBody))
	batchReq.Header.Set("Content-Type", "application/json")
	batchReq.AddCookie(adminCookie)
	batchRec := httptest.NewRecorder()
	router.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("create redeem batch failed (%d): %s", batchRec.Code, batchRec.Body.String())
	}
	var batchEnv kernel.Envelope[struct {
		Codes []string `json:"codes"`
	}]
	_ = json.Unmarshal(batchRec.Body.Bytes(), &batchEnv)
	if len(batchEnv.Data.Codes) != 2 {
		t.Fatalf("expected 2 generated redeem codes, got %d", len(batchEnv.Data.Codes))
	}
	code := batchEnv.Data.Codes[0]

	// 3. Regular user redeems code -> balance increases from 100 to 150 credits; second use of same code fails
	redeemPayload, _ := json.Marshal(map[string]string{"code": code})
	rReq := httptest.NewRequest(http.MethodPost, "/api/v1/credits/redeem", bytes.NewReader(redeemPayload))
	rReq.Header.Set("Content-Type", "application/json")
	rReq.AddCookie(userCookie)
	rRec := httptest.NewRecorder()
	router.ServeHTTP(rRec, rReq)
	if rRec.Code != http.StatusOK {
		t.Fatalf("redeem failed (%d): %s", rRec.Code, rRec.Body.String())
	}

	rDupReq := httptest.NewRequest(http.MethodPost, "/api/v1/credits/redeem", bytes.NewReader(redeemPayload))
	rDupReq.Header.Set("Content-Type", "application/json")
	rDupReq.AddCookie(userCookie)
	rDupRec := httptest.NewRecorder()
	router.ServeHTTP(rDupRec, rDupReq)
	if rDupRec.Code != http.StatusBadRequest {
		t.Fatalf("expected duplicate redeem to fail with 400, got %d", rDupRec.Code)
	}

	// 4. Create payment order via yingce.payment/v1 RPC and verify signed webhook callback settlement
	ordBody, _ := json.Marshal(map[string]string{
		"productId": "prod_starter_100",
		"configId":  "paycfg_mock_default",
	})
	ordReq := httptest.NewRequest(http.MethodPost, "/api/v1/payments/orders", bytes.NewReader(ordBody))
	ordReq.Header.Set("Content-Type", "application/json")
	ordReq.AddCookie(userCookie)
	ordRec := httptest.NewRecorder()
	router.ServeHTTP(ordRec, ordReq)
	if ordRec.Code != http.StatusOK {
		t.Fatalf("create payment order failed (%d): %s", ordRec.Code, ordRec.Body.String())
	}
	var ordEnv kernel.Envelope[struct {
		Order struct {
			OutTradeNo  string `json:"outTradeNo"`
			AmountCents int64  `json:"amountCents"`
		} `json:"order"`
	}]
	_ = json.Unmarshal(ordRec.Body.Bytes(), &ordEnv)
	outTradeNo := ordEnv.Data.Order.OutTradeNo
	amt := ordEnv.Data.Order.AmountCents

	sign := payment.SignCallback("mock_pay_hmac_secret", outTradeNo, amt, "paid")
	cbPayload, _ := json.Marshal(map[string]any{
		"outTradeNo":      outTradeNo,
		"providerTradeNo": "wx_trade_999",
		"status":          "paid",
		"amountCents":     amt,
		"sign":            sign,
	})
	cbReq := httptest.NewRequest(http.MethodPost, "/api/v1/payments/notify/official-payment-epay/paycfg_mock_default", bytes.NewReader(cbPayload))
	cbRec := httptest.NewRecorder()
	router.ServeHTTP(cbRec, cbReq)
	if cbRec.Code != http.StatusOK || cbRec.Body.String() != "success" {
		t.Fatalf("payment notify failed (%d): %s", cbRec.Code, cbRec.Body.String())
	}

	// 5. Verify final user credit balance = 100 (signup) + 50 (redeem) + 110 (starter pack 100+10 bonus) = 260 credits
	sessReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessReq.AddCookie(userCookie)
	sessRec := httptest.NewRecorder()
	router.ServeHTTP(sessRec, sessReq)
	var sessEnv kernel.Envelope[struct {
		Credits struct {
			BalanceMicrocredits int64 `json:"balanceMicrocredits"`
		} `json:"credits"`
	}]
	_ = json.Unmarshal(sessRec.Body.Bytes(), &sessEnv)
	if sessEnv.Data.Credits.BalanceMicrocredits != 260_000_000 {
		t.Fatalf("expected 260_000_000 microcredits, got %d", sessEnv.Data.Credits.BalanceMicrocredits)
	}
}

func registerUser(t *testing.T, router http.Handler, email, password string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register %s failed (%d): %s", email, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "canvas_session" {
			return c
		}
	}
	t.Fatalf("missing canvas_session cookie for %s", email)
	return nil
}

func TestMemoryCoordinatorRateLimitAndSlots(t *testing.T) {
	ctx := context.Background()
	c := platform.NewMemoryCoordinator()

	ok1, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	ok2, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	ok3, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	if !ok1 || !ok2 || ok3 {
		t.Fatalf("unexpected rate limit results: %v %v %v", ok1, ok2, ok3)
	}

	s1, _ := c.AcquireSlot(ctx, "worker", "w1", 1, time.Minute)
	s2, _ := c.AcquireSlot(ctx, "worker", "w2", 1, time.Minute)
	if !s1 || s2 {
		t.Fatalf("expected slot capacity=1 enforcement: s1=%v s2=%v", s1, s2)
	}
	_ = c.ReleaseSlot(ctx, "worker", "w1")
	s3, _ := c.AcquireSlot(ctx, "worker", "w2", 1, time.Minute)
	if !s3 {
		t.Fatalf("expected slot acquire after release")
	}
}

func TestOutboundSSRFGuard(t *testing.T) {
	if !outbound.IsDisallowedIP(net.ParseIP("127.0.0.1")) {
		t.Fatal("127.0.0.1 must be disallowed")
	}
	if !outbound.IsDisallowedIP(net.ParseIP("10.0.0.1")) {
		t.Fatal("10.0.0.1 must be disallowed")
	}
	if !outbound.IsDisallowedIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("169.254.169.254 link-local metadata IP must be disallowed")
	}
	if outbound.IsDisallowedIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 public IP must be allowed")
	}
}

func TestArchitectureLayering(t *testing.T) {
	forbidden := []string{
		"github.com/smartinbox17-droid/canvas/backend/internal/app",
		"github.com/smartinbox17-droid/canvas/backend/internal/service",
		"github.com/smartinbox17-droid/canvas/backend/internal/handler",
	}
	pkgs := []string{"../kernel", "../platform", "../outbound", "../model", "../database", "../repository", "../auth", "../storage", "../assets", "../protocol", "../generation", "../payment"}
	fset := token.NewFileSet()

	for _, dir := range pkgs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			full := filepath.Join(dir, e.Name())
			f, err := parser.ParseFile(fset, full, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", full, err)
			}
			for _, imp := range f.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					if impPath == bad {
						t.Fatalf("layer violation: %s imports %s", full, impPath)
					}
				}
			}
		}
	}
}
