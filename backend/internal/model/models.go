package model

import (
	"time"

	"gorm.io/gorm"
)

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusPolling   TaskStatus = "polling"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type ProviderCancelStatus string

const (
	ProviderCancelNone      ProviderCancelStatus = ""
	ProviderCancelPending   ProviderCancelStatus = "pending"
	ProviderCancelSucceeded ProviderCancelStatus = "cancelled"
	ProviderCancelFailed    ProviderCancelStatus = "failed"
)

type SchemaMigration struct {
	Version   int       `gorm:"primaryKey"`
	Name      string    `gorm:"size:160;not null"`
	Checksum  string    `gorm:"size:64;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

type User struct {
	ID           string    `json:"id" gorm:"primaryKey;size:36"`
	Email        string    `json:"email" gorm:"uniqueIndex;size:160;not null"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	Role         string    `json:"role" gorm:"size:24;not null;default:user"`
	GroupID      string    `json:"groupId,omitempty" gorm:"size:36;index"`
	Status       string    `json:"status" gorm:"size:24;not null;default:active"`
	ReferralCode string    `json:"referralCode,omitempty" gorm:"size:32;uniqueIndex"`
	InviterID    string    `json:"inviterId,omitempty" gorm:"size:36;index"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type AuthSession struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	UserID    string    `json:"userId" gorm:"index;size:36;not null"`
	TokenHash string    `json:"-" gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"index;not null"`
	IP        string    `json:"ip" gorm:"size:64"`
	UserAgent string    `json:"userAgent" gorm:"size:255"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreditAccount struct {
	UserID              string    `json:"userId" gorm:"primaryKey;size:36"`
	BalanceMicrocredits int64     `json:"balanceMicrocredits" gorm:"not null;default:0"`
	FrozenMicrocredits  int64     `json:"frozenMicrocredits" gorm:"not null;default:0"`
	Version             int64     `json:"version" gorm:"not null;default:1"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type CreditLedger struct {
	ID           string    `json:"id" gorm:"primaryKey;size:36"`
	UserID       string    `json:"userId" gorm:"index;size:36;not null"`
	Delta        int64     `json:"delta" gorm:"not null"`
	BalanceAfter int64     `json:"balanceAfter" gorm:"not null"`
	Kind         string    `json:"kind" gorm:"size:32;index;not null"`
	RefType      string    `json:"refType" gorm:"size:32"`
	RefID        string    `json:"refId" gorm:"size:64;index"`
	IdemKey      string    `json:"idemKey" gorm:"uniqueIndex;size:128;not null"`
	CreatedAt    time.Time `json:"createdAt"`
}

type BillingOrder struct {
	ID                  string    `json:"id" gorm:"primaryKey;size:36"`
	UserID              string    `json:"userId" gorm:"index;size:36;not null"`
	TaskID              string    `json:"taskId" gorm:"index;size:36;not null"`
	Status              string    `json:"status" gorm:"size:24;index;not null"`
	ReservedMicrocredit int64     `json:"reservedMicrocredits" gorm:"not null"`
	ActualMicrocredit   int64     `json:"actualMicrocredits" gorm:"not null;default:0"`
	PricingSnapshotJSON string    `json:"-" gorm:"type:text"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type CanvasProject struct {
	ID                 string     `json:"id" gorm:"primaryKey;size:36"`
	OwnerID            string     `json:"ownerId" gorm:"index;size:36;not null"`
	TeamID             string     `json:"teamId,omitempty" gorm:"index;size:36"`
	Title              string     `json:"title" gorm:"size:200;not null"`
	Revision           int64      `json:"revision" gorm:"not null;default:1"`
	SnapshotJSON       string     `json:"snapshotJson,omitempty" gorm:"type:text"`
	ShareToken         *string    `json:"shareToken,omitempty" gorm:"uniqueIndex;size:64"`
	EditLeaseOwner     string     `json:"editLeaseOwner,omitempty" gorm:"size:64"`
	EditLeaseExpiresAt *time.Time `json:"editLeaseExpiresAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type Task struct {
	AgentExecutionReady          bool                 `json:"-" gorm:"not null;default:false;<-:create"`
	AgentRunID                   string               `json:"agentRunId,omitempty" gorm:"size:36;index"`
	GenerationID                 string               `json:"generationId,omitempty" gorm:"size:36;index"`
	ApprovalID                   string               `json:"approvalId,omitempty" gorm:"size:160"`
	AuthorizedChargeMicrocredits int64                `json:"authorizedChargeMicrocredits,omitempty"`
	ExecutionDiagnosticJSON      string               `json:"-" gorm:"type:text"`
	CancellationSource           string               `json:"cancellationSource,omitempty" gorm:"size:40"`
	CancellationActorID          string               `json:"cancellationActorId,omitempty" gorm:"size:36"`
	CancellationRequestedAt      *time.Time           `json:"cancellationRequestedAt,omitempty"`
	CreationSubmissionID         *string              `json:"creationSubmissionId,omitempty" gorm:"size:36;uniqueIndex"`
	SubmissionHash               string               `json:"-" gorm:"size:64"`
	WorkbenchKind                string               `json:"workbenchKind,omitempty" gorm:"size:16;index"`
	ID                           string               `json:"id" gorm:"primaryKey;size:36"`
	UserID                       string               `json:"userId" gorm:"index;size:36;index:idx_tasks_user_created,priority:1;index:idx_tasks_user_project_created,priority:1"`
	TraceID                      string               `json:"-" gorm:"index;size:96"`
	RequestID                    string               `json:"-" gorm:"index;size:96"`
	ProjectID                    string               `json:"projectId" gorm:"index;size:80;index:idx_tasks_user_project_created,priority:2"`
	Type                         string               `json:"type" gorm:"index;size:64"`
	Status                       TaskStatus           `json:"status" gorm:"index;size:24;index:idx_tasks_status_created,priority:1;index:idx_tasks_claim,priority:1;index:idx_tasks_provider_cancel,priority:1"`
	Stage                        string               `json:"stage" gorm:"size:80"`
	Progress                     int                  `json:"progress"`
	Prompt                       string               `json:"prompt"`
	Operation                    string               `json:"operation" gorm:"size:64"`
	Provider                     string               `json:"provider" gorm:"size:64"`
	Model                        string               `json:"model" gorm:"size:120"`
	LogicalModelID               string               `json:"logicalModelId,omitempty" gorm:"size:36;index"`
	LogicalModelRevisionID       string               `json:"logicalModelRevisionId,omitempty" gorm:"size:36;index"`
	RouteID                      string               `json:"routeId,omitempty" gorm:"size:36;index"`
	ChannelModelID               string               `json:"channelModelId,omitempty" gorm:"size:36;index"`
	ProtocolBindingJSON          string               `json:"-" gorm:"type:text"`
	RouteRun                     int                  `json:"-" gorm:"index"`
	BillingOrderID               string               `json:"billingOrderId,omitempty" gorm:"index;size:36"`
	ProviderRequestID            string               `json:"providerRequestId,omitempty" gorm:"index;size:160"`
	ProviderCancelStatus         ProviderCancelStatus `json:"providerCancelStatus,omitempty" gorm:"index;size:24;index:idx_tasks_provider_cancel,priority:2"`
	ProviderCancelError          string               `json:"providerCancelError,omitempty" gorm:"type:text"`
	ProviderCancelAttempts       int                  `json:"providerCancelAttempts,omitempty"`
	ProviderCancelRequestedAt    *time.Time           `json:"providerCancelRequestedAt,omitempty"`
	ProviderCancelledAt          *time.Time           `json:"providerCancelledAt,omitempty"`
	ProviderCancelNextCheckAt    *time.Time           `json:"providerCancelNextCheckAt,omitempty" gorm:"index:idx_tasks_provider_cancel,priority:3"`
	PollStage                    string               `json:"pollStage,omitempty" gorm:"size:32"`
	NextPollAt                   *time.Time           `json:"nextPollAt,omitempty" gorm:"index"`
	LeaseOwner                   string               `json:"-" gorm:"index;size:120"`
	LeaseExpiresAt               *time.Time           `json:"-" gorm:"index;index:idx_tasks_claim,priority:2"`
	InputJSON                    string               `json:"inputJson" gorm:"type:text"`
	ResultJSON                   string               `json:"resultJson" gorm:"type:text"`
	MediaRecoveryJSON            string               `json:"-" gorm:"type:text"`
	MediaStage                   string               `json:"mediaStage,omitempty" gorm:"size:24"`
	TextDraft                    string               `json:"textDraft,omitempty" gorm:"type:text"`
	Error                        string               `json:"error"`
	Attempts                     int                  `json:"attempts"`
	StartedAt                    *time.Time           `json:"startedAt"`
	CompletedAt                  *time.Time           `json:"completedAt"`
	CreatedAt                    time.Time            `json:"createdAt" gorm:"index:idx_tasks_user_created,priority:2;index:idx_tasks_status_created,priority:2;index:idx_tasks_claim,priority:3;index:idx_tasks_user_project_created,priority:3"`
	CreatedAtEpoch               int64                `json:"-" gorm:"index:idx_tasks_created_epoch"`
	UpdatedAt                    time.Time            `json:"updatedAt"`
}

func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedAtEpoch == 0 {
		if t.CreatedAt.IsZero() {
			t.CreatedAtEpoch = time.Now().Unix()
		} else {
			t.CreatedAtEpoch = t.CreatedAt.Unix()
		}
	}
	return nil
}
