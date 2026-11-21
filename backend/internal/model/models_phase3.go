package model

import "time"

type PaymentProduct struct {
	ID                 string    `json:"id" gorm:"primaryKey;size:36"`
	Title              string    `json:"title" gorm:"size:120;not null"`
	AmountCents        int64     `json:"amountCents" gorm:"not null"`
	CreditMicrocredits int64     `json:"creditMicrocredits" gorm:"not null"`
	BonusMicrocredits  int64     `json:"bonusMicrocredits" gorm:"not null;default:0"`
	Enabled            bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type PaymentProviderConfig struct {
	ID              string    `json:"id" gorm:"primaryKey;size:36"`
	ProviderID      string    `json:"providerId" gorm:"index;size:64;not null"` // e.g. official-payment-alipay-page | mock-pay
	Label           string    `json:"label" gorm:"size:120;not null"`
	Command         string    `json:"-" gorm:"size:255"` // optional external RPC binary path; empty uses embedded RPC runner
	EncryptedConfig string    `json:"-" gorm:"type:text"`
	Enabled         bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type PaymentOrder struct {
	ID                 string     `json:"id" gorm:"primaryKey;size:36"`
	OutTradeNo         string     `json:"outTradeNo" gorm:"uniqueIndex;size:64;not null"`
	UserID             string     `json:"userId" gorm:"index;size:36;not null"`
	ProductID          string     `json:"productId" gorm:"size:36;not null"`
	ProviderID         string     `json:"providerId" gorm:"size:64;not null"`
	ConfigID           string     `json:"configId" gorm:"size:36;not null"`
	AmountCents        int64      `json:"amountCents" gorm:"not null"`
	CreditMicrocredits int64      `json:"creditMicrocredits" gorm:"not null"`
	Status             string     `json:"status" gorm:"index;size:24;not null;default:pending"` // pending | paid | closed | failed
	ProviderTradeNo    string     `json:"providerTradeNo,omitempty" gorm:"size:128"`
	CheckoutJSON       string     `json:"checkoutJson,omitempty" gorm:"type:text"`
	PaidAt             *time.Time `json:"paidAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt" gorm:"index"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type RedeemBatch struct {
	ID                 string     `json:"id" gorm:"primaryKey;size:36"`
	Name               string     `json:"name" gorm:"size:160;not null"`
	CreditMicrocredits int64      `json:"creditMicrocredits" gorm:"not null"`
	TotalCount         int        `json:"totalCount" gorm:"not null"`
	UsedCount          int        `json:"usedCount" gorm:"not null;default:0"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	CreatedBy          string     `json:"createdBy" gorm:"size:36"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type RedeemCode struct {
	Code               string     `json:"code" gorm:"primaryKey;size:48"`
	BatchID            string     `json:"batchId" gorm:"index;size:36;not null"`
	CreditMicrocredits int64      `json:"creditMicrocredits" gorm:"not null"`
	UsedByUserID       *string    `json:"usedByUserId,omitempty" gorm:"index;size:36"`
	UsedAt             *time.Time `json:"usedAt,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type ReferralCommission struct {
	ID                     string    `json:"id" gorm:"primaryKey;size:36"`
	InviterUserID          string    `json:"inviterUserId" gorm:"index;size:36;not null"`
	InviteeUserID          string    `json:"inviteeUserId" gorm:"index;size:36;not null"`
	PaymentOrderID         string    `json:"paymentOrderId" gorm:"uniqueIndex;size:36;not null"`
	CommissionMicrocredits int64     `json:"commissionMicrocredits" gorm:"not null"`
	CreatedAt              time.Time `json:"createdAt"`
}
