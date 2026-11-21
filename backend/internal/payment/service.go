package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"gorm.io/gorm"
)

const (
	RPCProtocolVersion     = "yingce.payment/v1"
	ReferralCommissionRate = int64(10) // 10% referral commission in microcredits
)

var (
	ErrProductNotFound  = errors.New("payment product not found")
	ErrProviderNotFound = errors.New("payment provider not found")
	ErrOrderNotFound    = errors.New("payment order not found")
	ErrInvalidRedeem    = errors.New("redeem code is invalid, expired, or already used")
	ErrVerifyFailed     = errors.New("payment notification signature verification failed")
)

type RPCRequest struct {
	Version   string              `json:"version"`
	Operation string              `json:"operation"` // validate_config | create_order | query_order | close_order | verify_notification | download_trade_bill
	Config    map[string]any      `json:"config,omitempty"`
	Request   any                 `json:"request,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Body      string              `json:"bodyBase64,omitempty"`
	BillDate  string              `json:"billDate,omitempty"`
}

type RPCResponse struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type CreateOrderData struct {
	CheckoutType string `json:"checkoutType"` // qrcode | redirect | form
	CodeURL      string `json:"codeUrl,omitempty"`
	RedirectURL  string `json:"redirectUrl,omitempty"`
	FormHTML     string `json:"formHtml,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

type QueryOrderData struct {
	Status          string `json:"status"` // pending | paid | closed
	PaidAt          string `json:"paidAt,omitempty"`
	ProviderTradeNo string `json:"providerTradeNo,omitempty"`
	AmountCents     int64  `json:"amountCents"`
}

type VerifyNotificationData struct {
	Verified        bool   `json:"verified"`
	OutTradeNo      string `json:"outTradeNo"`
	ProviderTradeNo string `json:"providerTradeNo,omitempty"`
	Status          string `json:"status"`
	AmountCents     int64  `json:"amountCents"`
	AckBody         string `json:"ackBody"`
}

type Service struct {
	db        *gorm.DB
	secretBox *platform.SecretBox
}

func NewService(db *gorm.DB, secretBox *platform.SecretBox) *Service {
	return &Service{db: db, secretBox: secretBox}
}

func (s *Service) BootstrapDefaults(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		products := []model.PaymentProduct{
			{
				ID:                 "prod_starter_100",
				Title:              "创作入门包 (100 积分)",
				AmountCents:        990,
				CreditMicrocredits: 100_000_000,
				BonusMicrocredits:  10_000_000,
				Enabled:            true,
			},
			{
				ID:                 "prod_pro_1000",
				Title:              "专业导演包 (1,000 积分 + 赠 200)",
				AmountCents:        8900,
				CreditMicrocredits: 1_000_000_000,
				BonusMicrocredits:  200_000_000,
				Enabled:            true,
			},
		}
		for _, p := range products {
			if err := tx.Save(&p).Error; err != nil {
				return err
			}
		}

		encCfg, err := s.secretBox.Encrypt(`{"merchantId":"mch_local_01","secret":"mock_pay_hmac_secret"}`)
		if err != nil {
			return err
		}
		prov := model.PaymentProviderConfig{
			ID:              "paycfg_mock_default",
			ProviderID:      "official-payment-epay",
			Label:           "本地扫码支付网关 (yingce.payment/v1 RPC)",
			EncryptedConfig: encCfg,
			Enabled:         true,
		}
		return tx.Save(&prov).Error
	})
}

// InvokeRPC executes a payment plugin operation via JSON-over-stdio subprocess (or embedded runner when Command=="").
func (s *Service) InvokeRPC(ctx context.Context, command string, req RPCRequest) (RPCResponse, error) {
	req.Version = RPCProtocolVersion
	if strings.TrimSpace(command) == "" {
		return invokeEmbeddedRPC(req)
	}

	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	payload, err := json.Marshal(req)
	if err != nil {
		return RPCResponse{}, err
	}
	cmd := exec.CommandContext(execCtx, command)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return RPCResponse{}, fmt.Errorf("payment rpc subprocess failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	var resp RPCResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return RPCResponse{}, fmt.Errorf("invalid payment rpc stdout json: %w", err)
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func invokeEmbeddedRPC(req RPCRequest) (RPCResponse, error) {
	secret, _ := req.Config["secret"].(string)
	if secret == "" {
		secret = "mock_pay_hmac_secret"
	}

	switch req.Operation {
	case "validate_config":
		data, _ := json.Marshal(map[string]any{"valid": true})
		return RPCResponse{OK: true, Data: data}, nil

	case "create_order":
		m, _ := toMap(req.Request)
		outTradeNo, _ := m["outTradeNo"].(string)
		data, _ := json.Marshal(CreateOrderData{
			CheckoutType: "qrcode",
			CodeURL:      fmt.Sprintf("weixin://wxpay/bizpayurl?pr=%s", outTradeNo),
			ExpiresAt:    time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
		})
		return RPCResponse{OK: true, Data: data}, nil

	case "query_order":
		m, _ := toMap(req.Request)
		outTradeNo, _ := m["outTradeNo"].(string)
		amt, _ := m["amountCents"].(float64)
		data, _ := json.Marshal(QueryOrderData{
			Status:          "paid",
			PaidAt:          time.Now().UTC().Format(time.RFC3339),
			ProviderTradeNo: "wx_" + outTradeNo,
			AmountCents:     int64(amt),
		})
		return RPCResponse{OK: true, Data: data}, nil

	case "close_order":
		data, _ := json.Marshal(map[string]any{"closed": true})
		return RPCResponse{OK: true, Data: data}, nil

	case "verify_notification":
		rawBody, err := base64.StdEncoding.DecodeString(req.Body)
		if err != nil {
			return RPCResponse{OK: false, Error: "invalid base64 body"}, err
		}
		var payload struct {
			OutTradeNo      string `json:"outTradeNo"`
			ProviderTradeNo string `json:"providerTradeNo"`
			Status          string `json:"status"`
			AmountCents     int64  `json:"amountCents"`
			Sign            string `json:"sign"`
		}
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			return RPCResponse{OK: false, Error: "invalid callback json"}, err
		}
		expected := SignCallback(secret, payload.OutTradeNo, payload.AmountCents, payload.Status)
		verified := hmac.Equal([]byte(expected), []byte(payload.Sign))
		data, _ := json.Marshal(VerifyNotificationData{
			Verified:        verified,
			OutTradeNo:      payload.OutTradeNo,
			ProviderTradeNo: payload.ProviderTradeNo,
			Status:          payload.Status,
			AmountCents:     payload.AmountCents,
			AckBody:         "success",
		})
		return RPCResponse{OK: true, Data: data}, nil

	case "download_trade_bill":
		data, _ := json.Marshal(map[string]any{"rows": []any{}})
		return RPCResponse{OK: true, Data: data}, nil

	default:
		return RPCResponse{OK: false, Error: "unsupported payment operation: " + req.Operation}, fmt.Errorf("unsupported operation %s", req.Operation)
	}
}

func SignCallback(secret, outTradeNo string, amountCents int64, status string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%s:%d:%s", outTradeNo, amountCents, status)
	return hex.EncodeToString(mac.Sum(nil))
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

func (s *Service) ListProducts(ctx context.Context) ([]model.PaymentProduct, error) {
	var list []model.PaymentProduct
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("amount_cents ASC").Find(&list).Error
	return list, err
}

func (s *Service) ListProviders(ctx context.Context) ([]model.PaymentProviderConfig, error) {
	var list []model.PaymentProviderConfig
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("created_at ASC").Find(&list).Error
	return list, err
}

func (s *Service) ListUserOrders(ctx context.Context, userID string) ([]model.PaymentOrder, error) {
	var list []model.PaymentOrder
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&list).Error
	return list, err
}

func (s *Service) CreateOrder(ctx context.Context, userID, productID, configID, notifyURL, returnURL, clientIP string) (*model.PaymentOrder, *CreateOrderData, error) {
	var prod model.PaymentProduct
	if err := s.db.WithContext(ctx).First(&prod, "id = ? AND enabled = ?", productID, true).Error; err != nil {
		return nil, nil, ErrProductNotFound
	}
	var cfg model.PaymentProviderConfig
	q := s.db.WithContext(ctx).Where("enabled = ?", true)
	if configID != "" {
		q = q.Where("id = ?", configID)
	}
	if err := q.First(&cfg).Error; err != nil {
		return nil, nil, ErrProviderNotFound
	}

	cfgMap := s.decryptProviderConfig(cfg.EncryptedConfig)
	outTradeNo := "YC" + time.Now().UTC().Format("20060102150405") + strings.ToUpper(uuid.NewString()[:6])

	rpcResp, err := s.InvokeRPC(ctx, cfg.Command, RPCRequest{
		Operation: "create_order",
		Config:    cfgMap,
		Request: map[string]any{
			"outTradeNo":  outTradeNo,
			"amountCents": prod.AmountCents,
			"subject":     prod.Title,
			"notifyUrl":   notifyURL,
			"returnUrl":   returnURL,
			"clientIp":    clientIP,
		},
	})
	if err != nil {
		return nil, nil, err
	}

	var checkout CreateOrderData
	if err := json.Unmarshal(rpcResp.Data, &checkout); err != nil {
		return nil, nil, err
	}

	order := &model.PaymentOrder{
		ID:                 uuid.NewString(),
		OutTradeNo:         outTradeNo,
		UserID:             userID,
		ProductID:          prod.ID,
		ProviderID:         cfg.ProviderID,
		ConfigID:           cfg.ID,
		AmountCents:        prod.AmountCents,
		CreditMicrocredits: prod.CreditMicrocredits + prod.BonusMicrocredits,
		Status:             "pending",
		CheckoutJSON:       string(rpcResp.Data),
	}
	if err := s.db.WithContext(ctx).Create(order).Error; err != nil {
		return nil, nil, err
	}
	return order, &checkout, nil
}

// HandleNotification verifies callback signature via RPC, double-checks via query_order, and atomically credits user + referral inviter.
func (s *Service) HandleNotification(ctx context.Context, providerID, configID string, headers map[string][]string, rawBody []byte) (string, error) {
	var cfg model.PaymentProviderConfig
	if err := s.db.WithContext(ctx).First(&cfg, "id = ? AND provider_id = ?", configID, providerID).Error; err != nil {
		return "", ErrProviderNotFound
	}
	cfgMap := s.decryptProviderConfig(cfg.EncryptedConfig)

	verifyResp, err := s.InvokeRPC(ctx, cfg.Command, RPCRequest{
		Operation: "verify_notification",
		Config:    cfgMap,
		Headers:   headers,
		Body:      base64.StdEncoding.EncodeToString(rawBody),
	})
	if err != nil {
		return "", err
	}
	var verified VerifyNotificationData
	if err := json.Unmarshal(verifyResp.Data, &verified); err != nil {
		return "", err
	}
	if !verified.Verified {
		return "", ErrVerifyFailed
	}

	var order model.PaymentOrder
	if err := s.db.WithContext(ctx).First(&order, "out_trade_no = ?", verified.OutTradeNo).Error; err != nil {
		return "", ErrOrderNotFound
	}
	if order.Status == "paid" {
		return verified.AckBody, nil
	}

	// Double-confirm amount and status via query_order RPC
	queryResp, err := s.InvokeRPC(ctx, cfg.Command, RPCRequest{
		Operation: "query_order",
		Config:    cfgMap,
		Request: map[string]any{
			"outTradeNo":  order.OutTradeNo,
			"amountCents": order.AmountCents,
		},
	})
	if err != nil {
		return "", err
	}
	var queried QueryOrderData
	if err := json.Unmarshal(queryResp.Data, &queried); err != nil {
		return "", err
	}
	if queried.Status != "paid" || queried.AmountCents != order.AmountCents {
		return "", errors.New("payment order amount or status mismatch on query confirmation")
	}

	err = s.settlePaidOrder(ctx, &order, queried.ProviderTradeNo)
	if err != nil {
		return "", err
	}
	return verified.AckBody, nil
}

func (s *Service) settlePaidOrder(ctx context.Context, order *model.PaymentOrder, providerTradeNo string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.PaymentOrder{}).
			Where("id = ? AND status = ?", order.ID, "pending").
			Updates(map[string]any{
				"status":            "paid",
				"provider_trade_no": providerTradeNo,
				"paid_at":           &now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Already settled concurrently
			return nil
		}

		if err := addCreditsInTx(tx, order.UserID, order.CreditMicrocredits, "recharge", "payment_order", order.ID, "recharge:"+order.OutTradeNo); err != nil {
			return err
		}

		// Referral commission if user has an inviter
		var buyer model.User
		if err := tx.First(&buyer, "id = ?", order.UserID).Error; err == nil && buyer.InviterID != "" {
			commission := (order.CreditMicrocredits * ReferralCommissionRate) / 100
			if commission > 0 {
				commRecord := model.ReferralCommission{
					ID:                     uuid.NewString(),
					InviterUserID:          buyer.InviterID,
					InviteeUserID:          buyer.ID,
					PaymentOrderID:         order.ID,
					CommissionMicrocredits: commission,
				}
				if err := tx.Create(&commRecord).Error; err != nil {
					return err
				}
				if err := addCreditsInTx(tx, buyer.InviterID, commission, "commission", "payment_order", order.ID, "commission:"+order.OutTradeNo); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func addCreditsInTx(tx *gorm.DB, userID string, delta int64, kind, refType, refID, idemKey string) error {
	var acc model.CreditAccount
	if err := tx.First(&acc, "user_id = ?", userID).Error; err != nil {
		return err
	}
	newBal := acc.BalanceMicrocredits + delta
	if err := tx.Model(&acc).Updates(map[string]any{
		"balance_microcredits": newBal,
		"version":              acc.Version + 1,
	}).Error; err != nil {
		return err
	}
	return tx.Create(&model.CreditLedger{
		ID:           uuid.NewString(),
		UserID:       userID,
		Delta:        delta,
		BalanceAfter: newBal,
		Kind:         kind,
		RefType:      refType,
		RefID:        refID,
		IdemKey:      idemKey,
	}).Error
}

func (s *Service) CreateRedeemBatch(ctx context.Context, adminUserID, name string, creditMicro int64, count int) (*model.RedeemBatch, []string, error) {
	if creditMicro <= 0 || count <= 0 || count > 500 {
		return nil, nil, errors.New("invalid redeem batch parameters")
	}
	batch := &model.RedeemBatch{
		ID:                 uuid.NewString(),
		Name:               strings.TrimSpace(name),
		CreditMicrocredits: creditMicro,
		TotalCount:         count,
		CreatedBy:          adminUserID,
	}
	if batch.Name == "" {
		batch.Name = "Redeem Batch"
	}
	codes := make([]string, 0, count)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			codeStr := "YC-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:12], "-", ""))
			codes = append(codes, codeStr)
			rc := model.RedeemCode{
				Code:               codeStr,
				BatchID:            batch.ID,
				CreditMicrocredits: creditMicro,
			}
			if err := tx.Create(&rc).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return batch, codes, nil
}

func (s *Service) Redeem(ctx context.Context, userID, rawCode string) (*model.CreditAccount, error) {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return nil, ErrInvalidRedeem
	}
	now := time.Now().UTC()

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rc model.RedeemCode
		if err := tx.First(&rc, "code = ?", code).Error; err != nil {
			return ErrInvalidRedeem
		}
		if rc.UsedByUserID != nil {
			return ErrInvalidRedeem
		}
		if rc.ExpiresAt != nil && now.After(*rc.ExpiresAt) {
			return ErrInvalidRedeem
		}

		res := tx.Model(&model.RedeemCode{}).
			Where("code = ? AND used_by_user_id IS NULL", code).
			Updates(map[string]any{
				"used_by_user_id": &userID,
				"used_at":         &now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInvalidRedeem
		}

		tx.Model(&model.RedeemBatch{}).
			Where("id = ?", rc.BatchID).
			Update("used_count", gorm.Expr("used_count + 1"))

		return addCreditsInTx(tx, userID, rc.CreditMicrocredits, "redeem", "redeem_code", rc.Code, "redeem:"+rc.Code)
	})
	if err != nil {
		return nil, err
	}

	var acc model.CreditAccount
	_ = s.db.WithContext(ctx).First(&acc, "user_id = ?", userID).Error
	return &acc, nil
}

type AdminOverview struct {
	TotalUsers           int64 `json:"totalUsers"`
	TotalTasks           int64 `json:"totalTasks"`
	SucceededTasks       int64 `json:"succeededTasks"`
	TotalBillingOrders   int64 `json:"totalBillingOrders"`
	PaidPaymentOrders    int64 `json:"paidPaymentOrders"`
	TotalRevenueCents    int64 `json:"totalRevenueCents"`
	TotalRedeemBatches   int64 `json:"totalRedeemBatches"`
}

func (s *Service) GetAdminOverview(ctx context.Context) (AdminOverview, error) {
	var ov AdminOverview
	db := s.db.WithContext(ctx)
	db.Model(&model.User{}).Count(&ov.TotalUsers)
	db.Model(&model.Task{}).Count(&ov.TotalTasks)
	db.Model(&model.Task{}).Where("status = ?", model.TaskStatusSucceeded).Count(&ov.SucceededTasks)
	db.Model(&model.BillingOrder{}).Count(&ov.TotalBillingOrders)
	db.Model(&model.PaymentOrder{}).Where("status = ?", "paid").Count(&ov.PaidPaymentOrders)
	db.Model(&model.PaymentOrder{}).Where("status = ?", "paid").Select("COALESCE(SUM(amount_cents), 0)").Scan(&ov.TotalRevenueCents)
	db.Model(&model.RedeemBatch{}).Count(&ov.TotalRedeemBatches)
	return ov, nil
}

func (s *Service) decryptProviderConfig(enc string) map[string]any {
	var out map[string]any
	if enc == "" {
		return map[string]any{}
	}
	if plain, err := s.secretBox.Decrypt(enc); err == nil {
		_ = json.Unmarshal([]byte(plain), &out)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out
}
