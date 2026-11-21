package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PutResult struct {
	StorageKey string
	URL        string
	SizeBytes  int64
	SHA256     string
}

type Provider interface {
	Put(ctx context.Context, key string, r io.Reader) (PutResult, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type LocalStore struct {
	rootDir   string
	urlPrefix string
}

func NewLocalStore(dataDir, urlPrefix string) (*LocalStore, error) {
	root := filepath.Join(dataDir, "uploads")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if urlPrefix == "" {
		urlPrefix = "/api/v1/assets/files"
	}
	return &LocalStore{rootDir: root, urlPrefix: strings.TrimRight(urlPrefix, "/")}, nil
}

func (l *LocalStore) safePath(key string) (string, error) {
	cleaned := filepath.Clean(strings.TrimLeft(key, "/\\"))
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", errors.New("invalid storage key")
	}
	full := filepath.Join(l.rootDir, cleaned)
	if !strings.HasPrefix(full, filepath.Clean(l.rootDir)+string(os.PathSeparator)) {
		return "", errors.New("storage path traversal blocked")
	}
	return full, nil
}

func (l *LocalStore) Put(_ context.Context, key string, r io.Reader) (PutResult, error) {
	full, err := l.safePath(key)
	if err != nil {
		return PutResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return PutResult{}, err
	}
	tmp := full + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return PutResult{}, err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), r)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return PutResult{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return PutResult{}, closeErr
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return PutResult{}, err
	}
	normalizedKey := filepath.ToSlash(strings.TrimLeft(filepath.Clean(key), "/\\"))
	return PutResult{
		StorageKey: normalizedKey,
		URL:        fmt.Sprintf("%s/%s", l.urlPrefix, normalizedKey),
		SizeBytes:  n,
		SHA256:     hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func (l *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.safePath(key)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (l *LocalStore) Delete(_ context.Context, key string) error {
	full, err := l.safePath(key)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
