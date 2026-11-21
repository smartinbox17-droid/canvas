package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"github.com/smartinbox17-droid/canvas/backend/internal/protocol"
	"gorm.io/gorm"
)

const (
	TaskLeaseTTL      = 45 * time.Second
	HeartbeatInterval = 15 * time.Second
)

var (
	ErrInsufficientCredits = errors.New("insufficient credits balance")
	ErrRouteUnavailable    = errors.New("no healthy route available for logical model")
	ErrTaskNotFound        = errors.New("task not found")
	ErrForbidden           = errors.New("access to task is forbidden")
)

type Service struct {
	db          *gorm.DB
	coord       platform.Coordinator
	secretBox   *platform.SecretBox
	httpClient  *http.Client
	workerID    string
	subscribers sync.Map // taskID -> map[chan model.TaskTextEvent]struct{}
}

func NewService(
	db *gorm.DB,
	coord platform.Coordinator,
	secretBox *platform.SecretBox,
	httpClient *http.Client,
) *Service {
	return &Service{
		db:         db,
		coord:      coord,
		secretBox:  secretBox,
		httpClient: httpClient,
		workerID:   "worker-" + uuid.NewString()[:8],
	}
}

func (s *Service) BootstrapMockCatalog(ctx context.Context, pluginManifestPath, mockBaseURL string) error {
	raw, err := os.ReadFile(filepath.Clean(pluginManifestPath))
	if err != nil {
		return err
	}
	var mf protocol.Manifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inst := model.PluginInstallation{
			ID:           mf.ID,
			Name:         mf.Name,
			Version:      mf.Version,
			ManifestJSON: string(raw),
			Enabled:      true,
		}
		if err := tx.Save(&inst).Error; err != nil {
			return err
		}

		encCfg, err := s.secretBox.Encrypt(`{"apiKey":"mock-dev-key"}`)
		if err != nil {
			return err
		}
		channel := model.ModelChannel{
			ID:               "ch_mock_default",
			Name:             "Built-in Mock Channel",
			PluginID:         mf.ID,
			ProviderID:       "mock",
			BaseURL:          mockBaseURL,
			EncryptedSecret:  encCfg,
			ConcurrencyLimit: 10,
			Enabled:          true,
		}
		if err := tx.Save(&channel).Error; err != nil {
			return err
		}

		for _, item := range []struct {
			code       string
			label      string
			capability string
			price      int64
		}{
			{"mock-gpt-4o", "Mock Text LLM", "text", 500_000},
			{"mock-flux-pro", "Mock Flux Image", "image", 1_000_000},
			{"mock-seedance-video", "Mock Seedance Video", "video", 2_500_000},
		} {
			cmID := "cm_" + item.code
			cm := model.ChannelModel{
				ID:            cmID,
				ChannelID:     channel.ID,
				UpstreamModel: item.code,
				Capability:    item.capability,
				Enabled:       true,
				Healthy:       true,
			}
			if err := tx.Save(&cm).Error; err != nil {
				return err
			}

			lmID := "lm_" + item.code
			revID := "rev_" + item.code
			lm := model.LogicalModel{
				ID:                lmID,
				Code:              item.code,
				Label:             item.label,
				Capability:        item.capability,
				ActiveRevisionID:  revID,
				DefaultPriceMicro: item.price,
				Enabled:           true,
			}
			if err := tx.Save(&lm).Error; err != nil {
				return err
			}
			rev := model.LogicalModelRevision{
				ID:             revID,
				LogicalModelID: lmID,
				RevisionNumber: 1,
			}
			if err := tx.Save(&rev).Error; err != nil {
				return err
			}
			route := model.LogicalModelRoute{
				ID:             "rt_" + item.code,
				LogicalModelID: lmID,
				RevisionID:     revID,
				ChannelID:      channel.ID,
				ChannelModelID: cmID,
				Priority:       1,
				Weight:         100,
				Enabled:        true,
			}
			if err := tx.Save(&route).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type SubmitTaskInput struct {
	UserID               string
	ProjectID            string
	CreationSubmissionID string
	LogicalModelCode     string
	Operation            string
	Request              protocol.GenerationRequest
}

type QuoteResult struct {
	EstimateMicrocredits int64  `json:"estimateMicrocredits"`
	LogicalModelID       string `json:"logicalModelId"`
	ModelCode            string `json:"modelCode"`
	Capability           string `json:"capability"`
}

func (s *Service) Quote(ctx context.Context, modelCode string, req protocol.GenerationRequest) (QuoteResult, error) {
	var lm model.LogicalModel
	err := s.db.WithContext(ctx).Where("code = ? AND enabled = ?", modelCode, true).First(&lm).Error
	if err != nil {
		return QuoteResult{}, ErrRouteUnavailable
	}
	count := int64(req.Output.Count)
	if count <= 0 {
		count = 1
	}
	dur := int64(req.Output.Duration)
	if dur <= 0 {
		dur = 1
	}
	price := lm.DefaultPriceMicro * count
	if lm.Capability == "video" {
		price = lm.DefaultPriceMicro * count * dur
	}
	return QuoteResult{
		EstimateMicrocredits: price,
		LogicalModelID:       lm.ID,
		ModelCode:            lm.Code,
		Capability:           lm.Capability,
	}, nil
}

func (s *Service) SubmitTask(ctx context.Context, in SubmitTaskInput) (*model.Task, error) {
	// Idempotent deduplication by CreationSubmissionID
	if in.CreationSubmissionID != "" {
		var existing model.Task
		err := s.db.WithContext(ctx).Where("creation_submission_id = ?", in.CreationSubmissionID).First(&existing).Error
		if err == nil {
			if existing.UserID != in.UserID {
				return nil, ErrForbidden
			}
			return &existing, nil
		}
	}

	modelCode := in.LogicalModelCode
	if modelCode == "" {
		modelCode = in.Request.Model
	}
	quote, err := s.Quote(ctx, modelCode, in.Request)
	if err != nil {
		return nil, err
	}

	reqBytes, err := json.Marshal(in.Request)
	if err != nil {
		return nil, err
	}

	var task model.Task
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var acc model.CreditAccount
		if err := tx.Where("user_id = ?", in.UserID).First(&acc).Error; err != nil {
			return ErrInsufficientCredits
		}
		available := acc.BalanceMicrocredits - acc.FrozenMicrocredits
		if available < quote.EstimateMicrocredits {
			return ErrInsufficientCredits
		}

		res := tx.Model(&model.CreditAccount{}).
			Where("user_id = ? AND version = ?", in.UserID, acc.Version).
			Updates(map[string]any{
				"frozen_microcredits": acc.FrozenMicrocredits + quote.EstimateMicrocredits,
				"version":             acc.Version + 1,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("concurrent credit update conflict, retry")
		}

		taskID := uuid.NewString()
		orderID := uuid.NewString()
		order := model.BillingOrder{
			ID:                  orderID,
			UserID:              in.UserID,
			TaskID:              taskID,
			Status:              "reserved",
			ReservedMicrocredit: quote.EstimateMicrocredits,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		ledger := model.CreditLedger{
			ID:           uuid.NewString(),
			UserID:       in.UserID,
			Delta:        -quote.EstimateMicrocredits,
			BalanceAfter: acc.BalanceMicrocredits - (acc.FrozenMicrocredits + quote.EstimateMicrocredits),
			Kind:         "reserve",
			RefType:      "billing_order",
			RefID:        orderID,
			IdemKey:      "reserve:" + taskID,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}

		var subID *string
		if in.CreationSubmissionID != "" {
			sub := in.CreationSubmissionID
			subID = &sub
		}
		task = model.Task{
			ID:                           taskID,
			UserID:                       in.UserID,
			ProjectID:                    in.ProjectID,
			CreationSubmissionID:         subID,
			Type:                         quote.Capability,
			Status:                       model.TaskStatusQueued,
			Stage:                        "queued",
			Progress:                     0,
			Prompt:                       in.Request.Prompt,
			Operation:                    in.Operation,
			Model:                        quote.ModelCode,
			LogicalModelID:               quote.LogicalModelID,
			BillingOrderID:               orderID,
			AuthorizedChargeMicrocredits: quote.EstimateMicrocredits,
			InputJSON:                    string(reqBytes),
		}
		return tx.Create(&task).Error
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ProcessNextTask claims one queued or expired-lease task and executes it with route failover and billing settlement.
func (s *Service) ProcessNextTask(ctx context.Context) (*model.Task, error) {
	now := time.Now().UTC()
	var task model.Task
	err := s.db.WithContext(ctx).
		Where("status = ? OR (status IN ? AND lease_expires_at < ?)",
			model.TaskStatusQueued,
			[]model.TaskStatus{model.TaskStatusRunning, model.TaskStatusPolling},
			now,
		).
		Order("created_at ASC").
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	leaseExp := now.Add(TaskLeaseTTL)
	started := now
	claimRes := s.db.WithContext(ctx).Model(&model.Task{}).
		Where("id = ? AND (status = ? OR lease_expires_at < ?)", task.ID, model.TaskStatusQueued, now).
		Updates(map[string]any{
			"status":           model.TaskStatusRunning,
			"stage":            "dispatching",
			"lease_owner":      s.workerID,
			"lease_expires_at": &leaseExp,
			"attempts":         task.Attempts + 1,
			"started_at":       &started,
		})
	if claimRes.Error != nil || claimRes.RowsAffected == 0 {
		return nil, claimRes.Error
	}
	_ = s.db.WithContext(ctx).First(&task, "id = ?", task.ID).Error

	if err := s.executeClaimedTask(ctx, &task); err != nil {
		_ = s.failAndRefundTask(ctx, &task, err.Error())
	}
	_ = s.db.WithContext(ctx).First(&task, "id = ?", task.ID).Error
	return &task, nil
}

func (s *Service) executeClaimedTask(ctx context.Context, task *model.Task) error {
	var req protocol.GenerationRequest
	if err := json.Unmarshal([]byte(task.InputJSON), &req); err != nil {
		return err
	}

	var routes []model.LogicalModelRoute
	if err := s.db.WithContext(ctx).
		Where("logical_model_id = ? AND enabled = ?", task.LogicalModelID, true).
		Find(&routes).Error; err != nil || len(routes) == 0 {
		return ErrRouteUnavailable
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority != routes[j].Priority {
			return routes[i].Priority < routes[j].Priority
		}
		return routes[i].Weight > routes[j].Weight
	})

	var lastErr error
	for _, rt := range routes {
		var cm model.ChannelModel
		if err := s.db.WithContext(ctx).First(&cm, "id = ? AND enabled = ? AND healthy = ?", rt.ChannelModelID, true, true).Error; err != nil {
			continue
		}
		var ch model.ModelChannel
		if err := s.db.WithContext(ctx).First(&ch, "id = ? AND enabled = ?", rt.ChannelID, true).Error; err != nil {
			continue
		}
		var plugin model.PluginInstallation
		if err := s.db.WithContext(ctx).First(&plugin, "id = ? AND enabled = ?", ch.PluginID, true).Error; err != nil {
			continue
		}
		var mf protocol.Manifest
		if err := json.Unmarshal([]byte(plugin.ManifestJSON), &mf); err != nil || len(mf.Contributes.Providers) == 0 {
			continue
		}
		provider := mf.Contributes.Providers[0]
		for _, p := range mf.Contributes.Providers {
			if p.ID == ch.ProviderID {
				provider = p
				break
			}
		}

		// Acquire channel concurrency slot
		acquired, _ := s.coord.AcquireSlot(ctx, "channel:"+ch.ID, task.ID, ch.ConcurrencyLimit, TaskLeaseTTL)
		if !acquired {
			continue
		}

		var cfgMap map[string]any
		if ch.EncryptedSecret != "" {
			if plain, err := s.secretBox.Decrypt(ch.EncryptedSecret); err == nil {
				_ = json.Unmarshal([]byte(plain), &cfgMap)
			}
		}
		reqForRoute := req
		reqForRoute.Model = cm.UpstreamModel
		scope := map[string]any{
			"request": protocol.RequestToScopeMap(reqForRoute),
			"config":  cfgMap,
		}

		rawResp, callErr := protocol.ExecuteOperation(ctx, s.httpClient, provider, ch.BaseURL, provider.Request.Create, scope)
		_ = s.coord.ReleaseSlot(ctx, "channel:"+ch.ID, task.ID)

		if callErr != nil {
			lastErr = callErr
			var upErr *protocol.UpstreamCallError
			if errors.As(callErr, &upErr) && !upErr.Retryable {
				// Non-retryable 4xx error: fail immediately without route failover
				return callErr
			}
			continue
		}

		norm, err := protocol.NormalizeResponse(provider.Response, rawResp, scope)
		if err != nil {
			return err
		}

		// Once upstream accepts and returns a taskId, NEVER failover to another route (anti-double-charge rule)
		if norm.Status == "pending" || norm.Status == "processing" {
			nextPoll := time.Now().UTC().Add(3 * time.Second)
			return s.db.WithContext(ctx).Model(task).
				Where("id = ? AND lease_owner = ?", task.ID, s.workerID).
				Updates(map[string]any{
					"status":              model.TaskStatusPolling,
					"stage":               "polling",
					"route_id":            rt.ID,
					"channel_model_id":    cm.ID,
					"provider":            provider.ID,
					"provider_request_id": norm.TaskID,
					"next_poll_at":        &nextPoll,
				}).Error
		}

		if norm.Status == "failed" {
			msg := norm.Message
			if msg == "" {
				msg = "upstream provider reported failure"
			}
			return errors.New(msg)
		}

		if norm.Text != "" {
			s.appendTextEvents(ctx, task.ID, norm.Text)
		}

		return s.succeedAndSettleTask(ctx, task, rt.ID, cm.ID, provider.ID, norm)
	}

	if lastErr != nil {
		return lastErr
	}
	return ErrRouteUnavailable
}

func (s *Service) succeedAndSettleTask(
	ctx context.Context,
	task *model.Task,
	routeID, channelModelID, providerID string,
	norm protocol.NormalizedResponse,
) error {
	resBytes, _ := json.Marshal(norm)
	now := time.Now().UTC()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		upd := tx.Model(&model.Task{}).
			Where("id = ? AND lease_owner = ?", task.ID, s.workerID).
			Updates(map[string]any{
				"status":              model.TaskStatusSucceeded,
				"stage":               "completed",
				"progress":            100,
				"route_id":            routeID,
				"channel_model_id":    channelModelID,
				"provider":            providerID,
				"provider_request_id": norm.TaskID,
				"result_json":         string(resBytes),
				"text_draft":          norm.Text,
				"completed_at":        &now,
			})
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected == 0 {
			return errors.New("lease fencing lost before settlement")
		}

		var order model.BillingOrder
		if err := tx.First(&order, "id = ?", task.BillingOrderID).Error; err != nil {
			return err
		}
		if order.Status != "reserved" {
			return nil
		}

		var acc model.CreditAccount
		if err := tx.First(&acc, "user_id = ?", task.UserID).Error; err != nil {
			return err
		}
		newBalance := acc.BalanceMicrocredits - order.ReservedMicrocredit
		newFrozen := acc.FrozenMicrocredits - order.ReservedMicrocredit
		if newFrozen < 0 {
			newFrozen = 0
		}
		if err := tx.Model(&acc).Updates(map[string]any{
			"balance_microcredits": newBalance,
			"frozen_microcredits":  newFrozen,
			"version":              acc.Version + 1,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&order).Updates(map[string]any{
			"status":             "settled",
			"actual_microcredit": order.ReservedMicrocredit,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.CreditLedger{
			ID:           uuid.NewString(),
			UserID:       task.UserID,
			Delta:        0,
			BalanceAfter: newBalance,
			Kind:         "settle",
			RefType:      "billing_order",
			RefID:        order.ID,
			IdemKey:      "settle:" + order.ID,
		}).Error
	})
}

func (s *Service) failAndRefundTask(ctx context.Context, task *model.Task, errMsg string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Model(&model.Task{}).
			Where("id = ? AND lease_owner = ?", task.ID, s.workerID).
			Updates(map[string]any{
				"status":       model.TaskStatusFailed,
				"stage":        "failed",
				"error":        errMsg,
				"completed_at": &now,
			})

		var order model.BillingOrder
		if err := tx.First(&order, "id = ?", task.BillingOrderID).Error; err != nil || order.Status != "reserved" {
			return nil
		}
		var acc model.CreditAccount
		if err := tx.First(&acc, "user_id = ?", task.UserID).Error; err != nil {
			return err
		}
		newFrozen := acc.FrozenMicrocredits - order.ReservedMicrocredit
		if newFrozen < 0 {
			newFrozen = 0
		}
		if err := tx.Model(&acc).Updates(map[string]any{
			"frozen_microcredits": newFrozen,
			"version":             acc.Version + 1,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&order).Update("status", "refunded").Error; err != nil {
			return err
		}
		return tx.Create(&model.CreditLedger{
			ID:           uuid.NewString(),
			UserID:       task.UserID,
			Delta:        order.ReservedMicrocredit,
			BalanceAfter: acc.BalanceMicrocredits,
			Kind:         "refund",
			RefType:      "billing_order",
			RefID:        order.ID,
			IdemKey:      "refund:" + order.ID,
		}).Error
	})
}

func (s *Service) appendTextEvents(ctx context.Context, taskID, fullText string) {
	chunks := splitChunks(fullText, 24)
	for idx, chunk := range chunks {
		seq := idx + 1
		payload, _ := json.Marshal(map[string]any{"text": chunk, "seq": seq})
		ev := model.TaskTextEvent{
			ID:       uuid.NewString(),
			TaskID:   taskID,
			Seq:      seq,
			Event:    "delta",
			DataJSON: string(payload),
		}
		_ = s.db.WithContext(ctx).Create(&ev).Error
		s.broadcastEvent(taskID, ev)
	}
	doneSeq := len(chunks) + 1
	donePayload, _ := json.Marshal(map[string]any{"status": "succeeded", "seq": doneSeq})
	doneEv := model.TaskTextEvent{
		ID:       uuid.NewString(),
		TaskID:   taskID,
		Seq:      doneSeq,
		Event:    "done",
		DataJSON: string(donePayload),
	}
	_ = s.db.WithContext(ctx).Create(&doneEv).Error
	s.broadcastEvent(taskID, doneEv)
}

func splitChunks(s string, size int) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return []string{""}
	}
	var out []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}

func (s *Service) ListTextEventsAfter(ctx context.Context, userID, taskID string, afterSeq int) ([]model.TaskTextEvent, error) {
	task, err := s.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	var events []model.TaskTextEvent
	err = s.db.WithContext(ctx).
		Where("task_id = ? AND seq > ?", task.ID, afterSeq).
		Order("seq ASC").
		Find(&events).Error
	return events, err
}

func (s *Service) broadcastEvent(taskID string, ev model.TaskTextEvent) {
	val, ok := s.subscribers.Load(taskID)
	if !ok {
		return
	}
	subs, _ := val.(*sync.Map)
	if subs == nil {
		return
	}
	subs.Range(func(key, _ any) bool {
		ch, ok := key.(chan model.TaskTextEvent)
		if ok {
			select {
			case ch <- ev:
			default:
			}
		}
		return true
	})
}

func (s *Service) GetTask(ctx context.Context, userID, taskID string) (*model.Task, error) {
	var t model.Task
	if err := s.db.WithContext(ctx).First(&t, "id = ?", taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if t.UserID != userID {
		return nil, ErrForbidden
	}
	return &t, nil
}

func (s *Service) ListTasks(ctx context.Context, userID string) ([]model.Task, error) {
	var list []model.Task
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(50).
		Find(&list).Error
	return list, err
}

func (s *Service) ListLogicalModels(ctx context.Context) ([]model.LogicalModel, error) {
	var list []model.LogicalModel
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("capability ASC, code ASC").Find(&list).Error
	return list, err
}

func (s *Service) CancelTask(ctx context.Context, userID, taskID string) (*model.Task, error) {
	t, err := s.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	if t.Status == model.TaskStatusSucceeded || t.Status == model.TaskStatusFailed || t.Status == model.TaskStatusCancelled {
		return t, nil
	}
	now := time.Now().UTC()
	cancelStatus := model.ProviderCancelNone
	if t.ProviderRequestID != "" {
		cancelStatus = model.ProviderCancelPending
	}
	_ = s.db.WithContext(ctx).Model(t).Updates(map[string]any{
		"status":                    model.TaskStatusCancelled,
		"stage":                     "cancelled",
		"cancellation_source":       "user",
		"cancellation_actor_id":     userID,
		"cancellation_requested_at": &now,
		"provider_cancel_status":    cancelStatus,
		"completed_at":              &now,
	}).Error
	_ = s.failAndRefundTask(ctx, t, "cancelled by user")
	_ = s.db.WithContext(ctx).First(t, "id = ?", taskID).Error
	return t, nil
}

func (s *Service) StartBackgroundWorkers(ctx context.Context, concurrency int) {
	if concurrency <= 0 {
		concurrency = 2
	}
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _ = s.ProcessNextTask(ctx)
				}
			}
		}(i)
	}
	go func() {
		_ = fmt.Sprintf("started %d generation workers", concurrency)
	}()
}
