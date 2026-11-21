package service

import (
	"github.com/smartinbox17-droid/canvas/backend/internal/app"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"gorm.io/gorm"
)

type Application = app.Application
type HealthStatus = app.HealthStatus

func NewApplication(cfg platform.Config, db *gorm.DB, coord platform.Coordinator) (*Application, error) {
	return app.New(cfg, db, coord)
}
