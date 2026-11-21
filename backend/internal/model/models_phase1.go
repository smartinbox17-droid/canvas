package model

import "time"

type SystemSetting struct {
	Key         string    `json:"key" gorm:"primaryKey;size:120"`
	ValueJSON   string    `json:"valueJson,omitempty" gorm:"type:text"`
	SecretValue string    `json:"-" gorm:"type:text"`
	IsSecret    bool      `json:"isSecret" gorm:"not null;default:false"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type LoginLog struct {
	ID         string    `json:"id" gorm:"primaryKey;size:36"`
	UserID     string    `json:"userId" gorm:"index;size:36"`
	Email      string    `json:"email" gorm:"index;size:160"`
	Method     string    `json:"method" gorm:"size:32"`
	Success    bool      `json:"success" gorm:"index"`
	FailReason string    `json:"failReason,omitempty" gorm:"size:160"`
	IP         string    `json:"ip" gorm:"size:64"`
	UserAgent  string    `json:"userAgent" gorm:"size:255"`
	CreatedAt  time.Time `json:"createdAt" gorm:"index"`
}

type AssetFolder struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID   string    `json:"ownerId" gorm:"index;size:36;not null"`
	ParentID  *string   `json:"parentId,omitempty" gorm:"index;size:36"`
	Name      string    `json:"name" gorm:"size:120;not null"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Asset struct {
	ID         string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID    string    `json:"ownerId" gorm:"index;size:36;not null"`
	FolderID   *string   `json:"folderId,omitempty" gorm:"index;size:36"`
	Name       string    `json:"name" gorm:"size:200;not null"`
	Kind       string    `json:"kind" gorm:"index;size:24;not null"` // image | video | audio | file
	MimeType   string    `json:"mimeType" gorm:"size:100;not null"`
	SizeBytes  int64     `json:"sizeBytes" gorm:"not null"`
	StorageKey string    `json:"-" gorm:"size:255;not null"`
	URL        string    `json:"url" gorm:"size:500;not null"`
	SHA256     string    `json:"sha256" gorm:"index;size:64"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	DurationMs int       `json:"durationMs,omitempty"`
	CreatedAt  time.Time `json:"createdAt" gorm:"index"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AssetReference struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	AssetID   string    `json:"assetId" gorm:"index;size:36;not null;uniqueIndex:idx_asset_ref_target"`
	RefType   string    `json:"refType" gorm:"size:32;not null;uniqueIndex:idx_asset_ref_target"` // canvas_node | task | drama_cast | drama_shot
	RefID     string    `json:"refId" gorm:"size:80;not null;uniqueIndex:idx_asset_ref_target"`
	CreatedAt time.Time `json:"createdAt"`
}

type StorageDeleteOutbox struct {
	ID         string     `json:"id" gorm:"primaryKey;size:36"`
	StorageKey string     `json:"storageKey" gorm:"size:255;not null"`
	Status     string     `json:"status" gorm:"index;size:24;not null;default:pending"` // pending | deleted | failed
	Attempts   int        `json:"attempts" gorm:"not null;default:0"`
	LastError  string     `json:"lastError,omitempty" gorm:"type:text"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt" gorm:"index"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
