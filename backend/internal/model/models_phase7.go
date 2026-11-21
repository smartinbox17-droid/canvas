package model

import "time"

type DramaProject struct {
	ID              string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID         string    `json:"ownerId" gorm:"index;size:36;not null"`
	Title           string    `json:"title" gorm:"size:200;not null"`
	RawScript       string    `json:"rawScript" gorm:"type:text"`
	StoryBibleJSON  string    `json:"storyBibleJson,omitempty" gorm:"type:text"`
	PacingPreset    string    `json:"pacingPreset" gorm:"size:32;not null;default:standard"` // fast_hook | standard | cinematic | dialogue | montage
	Stage           string    `json:"stage" gorm:"index;size:32;not null;default:script"`    // script | cast | segments | compose | completed
	Paused          bool      `json:"paused" gorm:"not null;default:false"`
	CanvasProjectID string    `json:"canvasProjectId,omitempty" gorm:"size:36;index"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type DramaCast struct {
	ID          string    `json:"id" gorm:"primaryKey;size:36"`
	DramaID     string    `json:"dramaId" gorm:"index;size:36;not null"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	RoleType    string    `json:"roleType" gorm:"size:32;not null;default:lead"` // lead | supporting | prop | scene
	Description string    `json:"description" gorm:"size:500"`
	ImagePrompt string    `json:"imagePrompt" gorm:"type:text"`
	ImageURL    string    `json:"imageUrl,omitempty" gorm:"size:500"`
	VoiceID     string    `json:"voiceId,omitempty" gorm:"size:64"`
	ImageReady  bool      `json:"imageReady" gorm:"not null;default:false"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type DramaEpisode struct {
	ID            string    `json:"id" gorm:"primaryKey;size:36"`
	DramaID       string    `json:"dramaId" gorm:"index;size:36;not null"`
	EpisodeNumber int       `json:"episodeNumber" gorm:"not null;default:1"`
	Title         string    `json:"title" gorm:"size:160;not null"`
	Summary       string    `json:"summary" gorm:"type:text"`
	Approved      bool      `json:"approved" gorm:"not null;default:false"`
	ComposedURL   string    `json:"composedUrl,omitempty" gorm:"size:500"`
	SRTContent    string    `json:"srtContent,omitempty" gorm:"type:text"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type DramaShot struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	DramaID           string    `json:"dramaId" gorm:"index;size:36;not null"`
	EpisodeID         string    `json:"episodeId" gorm:"index;size:36;not null"`
	ShotIndex         int       `json:"shotIndex" gorm:"not null"`
	DurationSec       int       `json:"durationSec" gorm:"not null;default:4"`
	ShotSize          string    `json:"shotSize" gorm:"size:40"`
	CameraMotion      string    `json:"cameraMotion" gorm:"size:80"`
	SceneDescription  string    `json:"sceneDescription" gorm:"type:text"`
	Dialogue          string    `json:"dialogue" gorm:"type:text"`
	ImagePrompt       string    `json:"imagePrompt" gorm:"type:text"`
	VideoMotionPrompt string    `json:"videoMotionPrompt" gorm:"type:text"`
	ContinuityOut     string    `json:"continuityOut" gorm:"size:300"`
	KeyframeURL       string    `json:"keyframeUrl,omitempty" gorm:"size:500"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type DramaSegment struct {
	ID             string    `json:"id" gorm:"primaryKey;size:36"`
	DramaID        string    `json:"dramaId" gorm:"index;size:36;not null"`
	EpisodeID      string    `json:"episodeId" gorm:"index;size:36;not null"`
	SegmentIndex   int       `json:"segmentIndex" gorm:"not null"`
	DurationSec    int       `json:"durationSec" gorm:"not null"`
	CombinedPrompt string    `json:"combinedPrompt" gorm:"type:text"`
	FirstFrameURL  string    `json:"firstFrameUrl,omitempty" gorm:"size:500"`
	TailFrameURL   string    `json:"tailFrameUrl,omitempty" gorm:"size:500"`
	TailFrameReady bool      `json:"tailFrameReady" gorm:"not null;default:false"`
	VideoURL       string    `json:"videoUrl,omitempty" gorm:"size:500"`
	Status         string    `json:"status" gorm:"index;size:24;not null;default:pending"` // pending | running | succeeded | failed
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
