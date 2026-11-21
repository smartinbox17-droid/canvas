package model

import "time"

type CanvasOperationLog struct {
	ID         string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID  string    `json:"projectId" gorm:"index:idx_canvas_op_rev,priority:1;size:36;not null"`
	Revision   int64     `json:"revision" gorm:"index:idx_canvas_op_rev,priority:2;not null"`
	ClientID   string    `json:"clientId" gorm:"size:64"`
	OpsJSON    string    `json:"opsJson" gorm:"type:text;not null"`
	CreatedAt  time.Time `json:"createdAt"`
}
