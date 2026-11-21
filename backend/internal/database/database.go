package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type MigrationStep struct {
	Version int
	Name    string
	Apply   func(tx *gorm.DB) error
}

var BuiltinMigrations = []MigrationStep{
	{
		Version: 1,
		Name:    "001_phase0_foundation_tables",
		Apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&model.User{},
				&model.AuthSession{},
				&model.CreditAccount{},
				&model.CreditLedger{},
				&model.BillingOrder{},
				&model.CanvasProject{},
				&model.Task{},
			)
		},
	},
	{
		Version: 2,
		Name:    "002_phase1_auth_storage_assets",
		Apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&model.SystemSetting{},
				&model.LoginLog{},
				&model.AssetFolder{},
				&model.Asset{},
				&model.AssetReference{},
				&model.StorageDeleteOutbox{},
			)
		},
	},
	{
		Version: 3,
		Name:    "003_phase2_protocol_channels_routes_tasks",
		Apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&model.PluginInstallation{},
				&model.ModelChannel{},
				&model.ChannelModel{},
				&model.LogicalModel{},
				&model.LogicalModelRevision{},
				&model.LogicalModelRoute{},
				&model.TaskTextEvent{},
			)
		},
	},
	{
		Version: 4,
		Name:    "004_phase3_billing_payments_redeem_admin",
		Apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&model.PaymentProduct{},
				&model.PaymentProviderConfig{},
				&model.PaymentOrder{},
				&model.RedeemBatch{},
				&model.RedeemCode{},
				&model.ReferralCommission{},
			)
		},
	},
	{
		Version: 5,
		Name:    "005_phase4_canvas_operation_logs",
		Apply: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&model.CanvasOperationLog{},
			)
		},
	},
}

func Open(cfg platform.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	switch cfg.DatabaseDriver {
	case "postgres", "postgresql", "pg":
		if cfg.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is required when CANVAS_DATABASE_DRIVER=postgres")
		}
		db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), gormCfg)
		if err != nil {
			return nil, err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(30)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		return db, nil

	case "sqlite", "":
		dsn := cfg.DatabaseURL
		if dsn == "" {
			if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
				return nil, err
			}
			dbPath := filepath.Join(cfg.DataDir, "canvas.db")
			dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", dbPath)
		}
		return gorm.Open(sqlite.Open(dsn), gormCfg)

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.DatabaseDriver)
	}
}

func stepChecksum(step MigrationStep) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", step.Version, step.Name)))
	return hex.EncodeToString(sum[:])
}

func MigrateUp(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.SchemaMigration{}); err != nil {
		return err
	}
	steps := make([]MigrationStep, len(BuiltinMigrations))
	copy(steps, BuiltinMigrations)
	sort.Slice(steps, func(i, j int) bool { return steps[i].Version < steps[j].Version })

	for _, step := range steps {
		expected := stepChecksum(step)
		var existing model.SchemaMigration
		err := db.First(&existing, "version = ?", step.Version).Error
		if err == nil {
			if existing.Checksum != expected {
				return fmt.Errorf("migration checksum mismatch at version %d: expected %s, got %s", step.Version, expected, existing.Checksum)
			}
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := step.Apply(tx); err != nil {
				return err
			}
			return tx.Create(&model.SchemaMigration{
				Version:   step.Version,
				Name:      step.Name,
				Checksum:  expected,
				AppliedAt: time.Now().UTC(),
			}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func VerifyMigrations(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.SchemaMigration{}) {
		return fmt.Errorf("schema_migrations table does not exist")
	}
	for _, step := range BuiltinMigrations {
		expected := stepChecksum(step)
		var existing model.SchemaMigration
		if err := db.First(&existing, "version = ?", step.Version).Error; err != nil {
			return fmt.Errorf("missing migration version %d (%s): %w", step.Version, step.Name, err)
		}
		if existing.Checksum != expected {
			return fmt.Errorf("checksum mismatch for version %d: expected %s, got %s", step.Version, expected, existing.Checksum)
		}
	}
	return nil
}
