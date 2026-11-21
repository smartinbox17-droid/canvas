package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	SessionCookieName = "canvas_session"
	SessionTTL        = 30 * 24 * time.Hour
	InitialBonusMicro = int64(100_000_000) // 100 credits initial signup bonus
)

var (
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrSessionExpired     = errors.New("session expired or invalid")
	ErrAccountDisabled    = errors.New("user account is disabled")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func generateOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) Register(ctx context.Context, email, password, ip, ua string) (*model.User, string, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	if normEmail == "" || len(password) < 8 {
		return nil, "", ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	var user model.User
	var rawToken string

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.User
		if err := tx.Where("email = ?", normEmail).First(&existing).Error; err == nil {
			return ErrEmailTaken
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// First registered user becomes admin automatically for self-hosted bootstrap
		var totalUsers int64
		if err := tx.Model(&model.User{}).Count(&totalUsers).Error; err != nil {
			return err
		}
		role := "user"
		if totalUsers == 0 {
			role = "admin"
		}

		user = model.User{
			ID:           uuid.NewString(),
			Email:        normEmail,
			PasswordHash: string(hash),
			Role:         role,
			Status:       "active",
			ReferralCode: strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		account := model.CreditAccount{
			UserID:              user.ID,
			BalanceMicrocredits: InitialBonusMicro,
			FrozenMicrocredits:  0,
			Version:             1,
		}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		ledger := model.CreditLedger{
			ID:           uuid.NewString(),
			UserID:       user.ID,
			Delta:        InitialBonusMicro,
			BalanceAfter: InitialBonusMicro,
			Kind:         "signup_bonus",
			RefType:      "user",
			RefID:        user.ID,
			IdemKey:      "signup_bonus:" + user.ID,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}

		tok, err := generateOpaqueToken()
		if err != nil {
			return err
		}
		rawToken = tok
		sess := model.AuthSession{
			ID:        uuid.NewString(),
			UserID:    user.ID,
			TokenHash: HashToken(rawToken),
			ExpiresAt: time.Now().UTC().Add(SessionTTL),
			IP:        ip,
			UserAgent: ua,
		}
		if err := tx.Create(&sess).Error; err != nil {
			return err
		}

		return tx.Create(&model.LoginLog{
			ID:        uuid.NewString(),
			UserID:    user.ID,
			Email:     user.Email,
			Method:    "register_password",
			Success:   true,
			IP:        ip,
			UserAgent: ua,
		}).Error
	})
	if err != nil {
		return nil, "", err
	}
	return &user, rawToken, nil
}

func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*model.User, string, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	var user model.User
	err := s.db.WithContext(ctx).Where("email = ?", normEmail).First(&user).Error
	if err != nil {
		_ = s.recordLoginLog(ctx, "", normEmail, "password", false, "user_not_found", ip, ua)
		return nil, "", ErrInvalidCredentials
	}
	if user.Status != "active" {
		_ = s.recordLoginLog(ctx, user.ID, normEmail, "password", false, "account_disabled", ip, ua)
		return nil, "", ErrAccountDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		_ = s.recordLoginLog(ctx, user.ID, normEmail, "password", false, "bad_password", ip, ua)
		return nil, "", ErrInvalidCredentials
	}

	rawToken, err := generateOpaqueToken()
	if err != nil {
		return nil, "", err
	}
	sess := model.AuthSession{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: HashToken(rawToken),
		ExpiresAt: time.Now().UTC().Add(SessionTTL),
		IP:        ip,
		UserAgent: ua,
	}
	if err := s.db.WithContext(ctx).Create(&sess).Error; err != nil {
		return nil, "", err
	}
	_ = s.recordLoginLog(ctx, user.ID, normEmail, "password", true, "", ip, ua)
	return &user, rawToken, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, rawToken string) (*model.User, *model.AuthSession, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, nil, ErrSessionExpired
	}
	var sess model.AuthSession
	err := s.db.WithContext(ctx).Where("token_hash = ?", HashToken(rawToken)).First(&sess).Error
	if err != nil {
		return nil, nil, ErrSessionExpired
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = s.db.WithContext(ctx).Delete(&sess).Error
		return nil, nil, ErrSessionExpired
	}
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", sess.UserID).Error; err != nil {
		return nil, nil, ErrSessionExpired
	}
	if user.Status != "active" {
		return nil, nil, ErrAccountDisabled
	}
	return &user, &sess, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	return s.db.WithContext(ctx).Where("token_hash = ?", HashToken(rawToken)).Delete(&model.AuthSession{}).Error
}

func (s *Service) GetCreditAccount(ctx context.Context, userID string) (*model.CreditAccount, error) {
	var acc model.CreditAccount
	if err := s.db.WithContext(ctx).First(&acc, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (s *Service) recordLoginLog(ctx context.Context, userID, email, method string, success bool, reason, ip, ua string) error {
	return s.db.WithContext(ctx).Create(&model.LoginLog{
		ID:         uuid.NewString(),
		UserID:     userID,
		Email:      email,
		Method:     method,
		Success:    success,
		FailReason: reason,
		IP:         ip,
		UserAgent:  ua,
	}).Error
}
