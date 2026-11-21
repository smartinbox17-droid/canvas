package app

import (
	"context"
	"sync/atomic"

	"github.com/smartinbox17-droid/canvas/backend/internal/assets"
	"github.com/smartinbox17-droid/canvas/backend/internal/auth"
	"github.com/smartinbox17-droid/canvas/backend/internal/database"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"github.com/smartinbox17-droid/canvas/backend/internal/repository"
	"github.com/smartinbox17-droid/canvas/backend/internal/storage"
	"gorm.io/gorm"
)

type Application struct {
	Config      platform.Config
	DB          *gorm.DB
	Repo        *repository.Repository
	Coordinator platform.Coordinator
	SecretBox   *platform.SecretBox
	Storage     storage.Provider
	Auth        *auth.Service
	Assets      *assets.Service
	ready       atomic.Bool
}

type HealthStatus struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

func New(cfg platform.Config, db *gorm.DB, coord platform.Coordinator) (*Application, error) {
	if cfg.AutoMigrate {
		if err := database.MigrateUp(db); err != nil {
			return nil, err
		}
	}
	secretBox, err := platform.LoadOrCreateSecretBox(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	localStore, err := storage.NewLocalStore(cfg.DataDir, "/api/v1/assets/files")
	if err != nil {
		return nil, err
	}

	a := &Application{
		Config:      cfg,
		DB:          db,
		Repo:        repository.New(db),
		Coordinator: coord,
		SecretBox:   secretBox,
		Storage:     localStore,
		Auth:        auth.NewService(db),
		Assets:      assets.NewService(db, localStore),
	}
	a.ready.Store(true)
	return a, nil
}

func (a *Application) IsReady() bool {
	return a.ready.Load()
}

func (a *Application) SetReady(ready bool) {
	a.ready.Store(ready)
}

func (a *Application) CheckReadiness(ctx context.Context) (HealthStatus, error) {
	status := HealthStatus{
		Status:   "ok",
		Database: "ok",
		Redis:    "ok",
	}
	if !a.IsReady() {
		status.Status = "starting"
	}
	if err := a.Repo.Ping(ctx); err != nil {
		status.Status = "degraded"
		status.Database = err.Error()
		return status, err
	}
	if err := database.VerifyMigrations(a.DB); err != nil {
		status.Status = "degraded"
		status.Database = err.Error()
		return status, err
	}
	if a.Coordinator != nil {
		if err := a.Coordinator.Ping(ctx); err != nil {
			status.Status = "degraded"
			status.Redis = err.Error()
			return status, err
		}
	}
	return status, nil
}

func (a *Application) Close() error {
	a.ready.Store(false)
	if a.Coordinator != nil {
		_ = a.Coordinator.Close()
	}
	if a.DB != nil {
		if sqlDB, err := a.DB.DB(); err == nil {
			return sqlDB.Close()
		}
	}
	return nil
}
