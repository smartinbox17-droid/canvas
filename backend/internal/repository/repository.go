package repository

import (
	"context"

	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *gorm.DB {
	return r.db
}

func (r *Repository) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Count(&count).Error
	return count, err
}

func (r *Repository) ListCanvasProjectsByOwner(ctx context.Context, ownerID string) ([]model.CanvasProject, error) {
	var projects []model.CanvasProject
	err := r.db.WithContext(ctx).
		Where("owner_id = ?", ownerID).
		Order("updated_at DESC").
		Find(&projects).Error
	return projects, err
}
