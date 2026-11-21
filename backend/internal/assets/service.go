package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartinbox17-droid/canvas/backend/internal/model"
	"github.com/smartinbox17-droid/canvas/backend/internal/storage"
	"gorm.io/gorm"
)

var (
	ErrAssetNotFound = errors.New("asset not found")
	ErrForbidden     = errors.New("access to asset is forbidden")
	ErrAssetInUse    = errors.New("asset is referenced by active canvas/task/drama resources and cannot be deleted")
)

type Service struct {
	db    *gorm.DB
	store storage.Provider
}

func NewService(db *gorm.DB, store storage.Provider) *Service {
	return &Service{db: db, store: store}
}

func classifyMimeKind(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func (s *Service) CreateFolder(ctx context.Context, ownerID, name string, parentID *string) (*model.AssetFolder, error) {
	folder := &model.AssetFolder{
		ID:       uuid.NewString(),
		OwnerID:  ownerID,
		ParentID: parentID,
		Name:     strings.TrimSpace(name),
	}
	if folder.Name == "" {
		folder.Name = "Untitled Folder"
	}
	if err := s.db.WithContext(ctx).Create(folder).Error; err != nil {
		return nil, err
	}
	return folder, nil
}

func (s *Service) ListFolders(ctx context.Context, ownerID string) ([]model.AssetFolder, error) {
	var folders []model.AssetFolder
	err := s.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("created_at ASC").Find(&folders).Error
	return folders, err
}

func (s *Service) Upload(ctx context.Context, ownerID, filename string, folderID *string, r io.Reader) (*model.Asset, error) {
	// Read up to 512 bytes for magic-number MIME detection
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	head = head[:n]
	mimeType := http.DetectContentType(head)
	kind := classifyMimeKind(mimeType)

	id := uuid.NewString()
	ext := filepath.Ext(filename)
	if len(ext) > 10 {
		ext = ""
	}
	storageKey := fmt.Sprintf("%s/%s/%s%s", ownerID, time.Now().UTC().Format("20060102"), id, ext)

	combined := io.MultiReader(bytes.NewReader(head), r)
	putRes, err := s.store.Put(ctx, storageKey, combined)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(filename)
	if name == "" {
		name = id + ext
	}
	asset := &model.Asset{
		ID:         id,
		OwnerID:    ownerID,
		FolderID:   folderID,
		Name:       name,
		Kind:       kind,
		MimeType:   mimeType,
		SizeBytes:  putRes.SizeBytes,
		StorageKey: putRes.StorageKey,
		URL:        fmt.Sprintf("/api/v1/assets/%s/file", id),
		SHA256:     putRes.SHA256,
	}
	if err := s.db.WithContext(ctx).Create(asset).Error; err != nil {
		_ = s.store.Delete(ctx, putRes.StorageKey)
		return nil, err
	}
	return asset, nil
}

func (s *Service) ListAssets(ctx context.Context, ownerID string, folderID *string, kind string) ([]model.Asset, error) {
	q := s.db.WithContext(ctx).Where("owner_id = ?", ownerID)
	if folderID != nil && *folderID != "" {
		q = q.Where("folder_id = ?", *folderID)
	}
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var list []model.Asset
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

func (s *Service) GetOwnedAsset(ctx context.Context, ownerID, assetID string) (*model.Asset, error) {
	var asset model.Asset
	if err := s.db.WithContext(ctx).First(&asset, "id = ?", assetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	if asset.OwnerID != ownerID {
		return nil, ErrForbidden
	}
	return &asset, nil
}

func (s *Service) OpenAssetFile(ctx context.Context, ownerID, assetID string) (*model.Asset, io.ReadCloser, error) {
	asset, err := s.GetOwnedAsset(ctx, ownerID, assetID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.Open(ctx, asset.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return asset, rc, nil
}

func (s *Service) BindReference(ctx context.Context, assetID, refType, refID string) error {
	ref := model.AssetReference{
		ID:      uuid.NewString(),
		AssetID: assetID,
		RefType: refType,
		RefID:   refID,
	}
	return s.db.WithContext(ctx).Where("asset_id = ? AND ref_type = ? AND ref_id = ?", assetID, refType, refID).
		FirstOrCreate(&ref).Error
}

func (s *Service) DeleteAsset(ctx context.Context, ownerID, assetID string) error {
	asset, err := s.GetOwnedAsset(ctx, ownerID, assetID)
	if err != nil {
		return err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var refCount int64
		if err := tx.Model(&model.AssetReference{}).Where("asset_id = ?", asset.ID).Count(&refCount).Error; err != nil {
			return err
		}
		if refCount > 0 {
			return ErrAssetInUse
		}
		if err := tx.Delete(asset).Error; err != nil {
			return err
		}
		outbox := model.StorageDeleteOutbox{
			ID:         uuid.NewString(),
			StorageKey: asset.StorageKey,
			Status:     "pending",
		}
		return tx.Create(&outbox).Error
	})
	if err != nil {
		return err
	}

	// Best-effort immediate physical GC cleanup
	_ = s.ProcessDeleteOutbox(ctx, 10)
	return nil
}

func (s *Service) ProcessDeleteOutbox(ctx context.Context, batchSize int) int {
	var items []model.StorageDeleteOutbox
	if err := s.db.WithContext(ctx).Where("status = ?", "pending").Order("created_at ASC").Limit(batchSize).Find(&items).Error; err != nil {
		return 0
	}
	cleaned := 0
	for _, item := range items {
		if err := s.store.Delete(ctx, item.StorageKey); err != nil {
			_ = s.db.WithContext(ctx).Model(&item).Updates(map[string]any{
				"attempts":   item.Attempts + 1,
				"last_error": err.Error(),
			}).Error
			continue
		}
		now := time.Now().UTC()
		_ = s.db.WithContext(ctx).Model(&item).Updates(map[string]any{
			"status":     "deleted",
			"deleted_at": &now,
		}).Error
		cleaned++
	}
	return cleaned
}
