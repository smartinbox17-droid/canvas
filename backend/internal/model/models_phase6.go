package model

import "time"

type Skill struct {
	ID            string    `json:"id" gorm:"primaryKey;size:64"`
	OwnerID       string    `json:"ownerId,omitempty" gorm:"index;size:36"` // empty = builtin system skill
	Name          string    `json:"name" gorm:"size:160;not null"`
	Category      string    `json:"category" gorm:"index;size:32;not null"`   // writing | storyboard | image | video | utility
	OutputMode    string    `json:"outputMode" gorm:"size:32;not null"`       // text | json | image_prompt | workflow
	Description   string    `json:"description" gorm:"size:500"`
	SystemPrompt  string    `json:"systemPrompt" gorm:"type:text;not null"`
	FilesJSON     string    `json:"filesJson,omitempty" gorm:"type:text"`     // multi-file Skill IDE content
	LikesCount    int       `json:"likesCount" gorm:"not null;default:0"`
	IsBuiltin     bool      `json:"isBuiltin" gorm:"index;not null;default:false"`
	Enabled       bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CloudAgentConversation struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	UserID    string    `json:"userId" gorm:"index;size:36;not null"`
	ProjectID string    `json:"projectId,omitempty" gorm:"index;size:36"`
	Title     string    `json:"title" gorm:"size:200;not null"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CloudAgentRun struct {
	ID             string     `json:"id" gorm:"primaryKey;size:36"`
	ConversationID string     `json:"conversationId" gorm:"index;size:36;not null"`
	UserID         string     `json:"userId" gorm:"index;size:36;not null"`
	ProjectID      string     `json:"projectId,omitempty" gorm:"index;size:36"`
	Prompt         string     `json:"prompt" gorm:"type:text;not null"`
	Model          string     `json:"model" gorm:"size:120"`
	Status         string     `json:"status" gorm:"index;size:32;not null"` // running | waiting_approval | completed | failed | cancelled | undone
	ToolBudget     int        `json:"toolBudget" gorm:"not null;default:16"`
	InverseOpsJSON string     `json:"-" gorm:"type:text"` // recorded inverse canvas operations for one-click run undo
	Error          string     `json:"error,omitempty" gorm:"type:text"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"index"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CloudAgentEvent struct {
	ID          string    `json:"id" gorm:"primaryKey;size:36"`
	RunID       string    `json:"runId" gorm:"index:idx_agent_run_seq,priority:1;size:36;not null"`
	Seq         int       `json:"seq" gorm:"index:idx_agent_run_seq,priority:2;not null"`
	Type        string    `json:"type" gorm:"size:48;not null"` // run.started | message.delta | tool.call | tool.result | approval.required | approval.resolved | canvas.ops | run.completed | run.undone
	PayloadJSON string    `json:"payloadJson" gorm:"type:text;not null"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CloudAgentApproval struct {
	ID             string     `json:"id" gorm:"primaryKey;size:36"`
	RunID          string     `json:"runId" gorm:"index;size:36;not null"`
	UserID         string     `json:"userId" gorm:"index;size:36;not null"`
	ToolName       string     `json:"toolName" gorm:"size:80;not null"`
	ArgsJSON       string     `json:"argsJson" gorm:"type:text;not null"`
	Status         string     `json:"status" gorm:"index;size:24;not null;default:pending"` // pending | approved | rejected
	Decision       string     `json:"decision,omitempty" gorm:"size:24"`
	EditedArgsJSON string     `json:"editedArgsJson,omitempty" gorm:"type:text"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type AgentMemoryDocument struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	UserID    string    `json:"userId" gorm:"index;size:36;not null"`
	Scope     string    `json:"scope" gorm:"index;size:32;not null;default:global"` // global | project
	Title     string    `json:"title" gorm:"size:160;not null"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MCPServerConfig struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	OwnerID   string    `json:"ownerId" gorm:"index;size:36;not null"`
	Name      string    `json:"name" gorm:"size:120;not null"`
	Transport string    `json:"transport" gorm:"size:32;not null;default:streamable_http"` // streamable_http | sse
	Endpoint  string    `json:"endpoint" gorm:"size:300;not null"`
	ToolsJSON string    `json:"toolsJson,omitempty" gorm:"type:text"`
	Enabled   bool      `json:"enabled" gorm:"index;not null;default:true"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
