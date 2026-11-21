package model

import "time"

type PluginInstallation struct {
	ID            string    `json:"id" gorm:"primaryKey;size:80"` // plugin id e.g. "mock-echo" or "openai-videos"
	Name          string    `json:"name" gorm:"size:160;not null"`
	Version       string    `json:"version" gorm:"size:32;not null"`
	ManifestJSON  string    `json:"manifestJson" gorm:"type:text;not null"`
	PackageSHA256 string    `json:"packageSha256" gorm:"size:64"`
	Enabled       bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ModelChannel struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	Name              string    `json:"name" gorm:"size:160;not null"`
	PluginID          string    `json:"pluginId" gorm:"index;size:80;not null"`
	ProviderID        string    `json:"providerId" gorm:"index;size:80;not null"`
	BaseURL           string    `json:"baseUrl" gorm:"size:300;not null"`
	EncryptedSecret   string    `json:"-" gorm:"type:text"` // AES-256-GCM encrypted JSON config (e.g. {"apiKey":"..."})
	ConcurrencyLimit  int       `json:"concurrencyLimit" gorm:"not null;default:3"`
	Enabled           bool      `json:"enabled" gorm:"index;not null;default:true"`
	ConsecutiveErrors int       `json:"consecutiveErrors" gorm:"not null;default:0"`
	CircuitOpenUntil  *time.Time `json:"circuitOpenUntil,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type ChannelModel struct {
	ID            string    `json:"id" gorm:"primaryKey;size:36"`
	ChannelID     string    `json:"channelId" gorm:"index;size:36;not null"`
	UpstreamModel string    `json:"upstreamModel" gorm:"size:120;not null"`
	Capability    string    `json:"capability" gorm:"index;size:32;not null"` // text | image | video | audio
	Enabled       bool      `json:"enabled" gorm:"index;not null;default:true"`
	Healthy       bool      `json:"healthy" gorm:"not null;default:true"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type LogicalModel struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	Code              string    `json:"code" gorm:"uniqueIndex;size:120;not null"` // e.g. "seedance-1.0-pro" or "gpt-image-1"
	Label             string    `json:"label" gorm:"size:160;not null"`
	Capability        string    `json:"capability" gorm:"index;size:32;not null"`
	ActiveRevisionID  string    `json:"activeRevisionId" gorm:"size:36"`
	DefaultPriceMicro int64     `json:"defaultPriceMicro" gorm:"not null;default:1000000"` // 1 credit default
	Enabled           bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type LogicalModelRevision struct {
	ID             string    `json:"id" gorm:"primaryKey;size:36"`
	LogicalModelID string    `json:"logicalModelId" gorm:"index;size:36;not null"`
	RevisionNumber int       `json:"revisionNumber" gorm:"not null;default:1"`
	CreatedAt      time.Time `json:"createdAt"`
}

type LogicalModelRoute struct {
	ID             string    `json:"id" gorm:"primaryKey;size:36"`
	LogicalModelID string    `json:"logicalModelId" gorm:"index;size:36;not null"`
	RevisionID     string    `json:"revisionId" gorm:"index;size:36;not null"`
	ChannelID      string    `json:"channelId" gorm:"index;size:36;not null"`
	ChannelModelID string    `json:"channelModelId" gorm:"index;size:36;not null"`
	Priority       int       `json:"priority" gorm:"not null;default:1"` // ascending (1 = highest priority)
	Weight         int       `json:"weight" gorm:"not null;default:100"`
	Enabled        bool      `json:"enabled" gorm:"not null;default:true"`
	CreatedAt      time.Time `json:"createdAt"`
}

type TaskTextEvent struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	TaskID    string    `json:"taskId" gorm:"index:idx_task_text_seq,priority:1;size:36;not null"`
	Seq       int       `json:"seq" gorm:"index:idx_task_text_seq,priority:2;not null"`
	Event     string    `json:"event" gorm:"size:32;not null"` // delta | done | error
	DataJSON  string    `json:"dataJson" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"createdAt"`
}
