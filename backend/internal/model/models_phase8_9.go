package model

import "time"

type EcommerceProject struct {
	ID              string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID         string    `json:"ownerId" gorm:"index;size:36;not null"`
	ProductName     string    `json:"productName" gorm:"size:160;not null"`
	CategoryCode    string    `json:"categoryCode" gorm:"size:64;not null"`
	TargetPlatform  string    `json:"targetPlatform" gorm:"index;size:48;not null"` // amazon | tiktok_shop | shopify | tmall | jd | xiaohongshu
	RegionCasting   string    `json:"regionCasting" gorm:"size:48;not null;default:global"`
	SafeAreaMargin  string    `json:"safeAreaMargin" gorm:"size:32;not null;default:8%-92%"`
	ComplianceScore int       `json:"complianceScore" gorm:"not null;default:100"`
	DirectionsJSON  string    `json:"directionsJson,omitempty" gorm:"type:text"`
	SelectedDirID   string    `json:"selectedDirId,omitempty" gorm:"size:64"`
	ImagesJSON      string    `json:"imagesJson,omitempty" gorm:"type:text"`
	Status          string    `json:"status" gorm:"size:32;not null;default:ready"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type EcommerceKnowledgeEntry struct {
	ID        string    `json:"id" gorm:"primaryKey;size:64"`
	Platform  string    `json:"platform" gorm:"index;size:48;not null"`
	Category  string    `json:"category" gorm:"index;size:64;not null"`
	RuleTitle string    `json:"ruleTitle" gorm:"size:200;not null"`
	RuleBody  string    `json:"ruleBody" gorm:"type:text;not null"`
	SafeZone  string    `json:"safeZone" gorm:"size:32;not null;default:8%-92%"`
	CreatedAt time.Time `json:"createdAt"`
}

type PrevisScene struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID           string    `json:"ownerId" gorm:"index;size:36;not null"`
	Title             string    `json:"title" gorm:"size:160;not null"`
	CityPreset        string    `json:"cityPreset" gorm:"size:48;not null"`        // cyberpunk_blocks | modern_cbd | ancient_courtyard | desert_highway | sci_fi_hangar | harbor_docks | suburban_street
	Weather           string    `json:"weather" gorm:"size:32;not null"`           // clear | rain | fog | snow | dusk | neon_night | sandstorm
	CameraRecipe      string    `json:"cameraRecipe" gorm:"size:64;not null"`      // dolly_zoom | crane_up | orbit_360 | low_angle_track | over_shoulder
	CharacterPose     string    `json:"characterPose" gorm:"size:64;not null"`     // action_sprint | combat_ready | umbrella_stand | look_back
	PropsJSON         string    `json:"propsJson,omitempty" gorm:"type:text"`      // parametric 3D props & A/B ground markers
	DepthVideoURL     string    `json:"depthVideoUrl,omitempty" gorm:"size:500"`
	WhiteModelVideo   string    `json:"whiteModelVideo,omitempty" gorm:"size:500"`
	CanvasNodeID      string    `json:"canvasNodeId,omitempty" gorm:"size:64"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type Team struct {
	ID                  string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID             string    `json:"ownerId" gorm:"index;size:36;not null"`
	ParentTeamID        *string   `json:"parentTeamId,omitempty" gorm:"index;size:36"` // supports 2-level hierarchy: Company -> Group
	Name                string    `json:"name" gorm:"size:160;not null"`
	PoolMicrocredits    int64     `json:"poolMicrocredits" gorm:"not null;default:0"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type TeamMembership struct {
	ID                    string    `json:"id" gorm:"primaryKey;size:36"`
	TeamID                string    `json:"teamId" gorm:"index:idx_team_member,priority:1;size:36;not null"`
	UserID                string    `json:"userId" gorm:"index:idx_team_member,priority:2;size:36;not null"`
	Role                  string    `json:"role" gorm:"size:24;not null;default:member"` // owner | admin | member
	QuotaMicrocredits     int64     `json:"quotaMicrocredits" gorm:"not null;default:0"`
	UsedQuotaMicrocredits int64     `json:"usedQuotaMicrocredits" gorm:"not null;default:0"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}
