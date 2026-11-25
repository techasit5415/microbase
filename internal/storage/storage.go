package storage

import (
	"context"
	"io"
	"sync"
)

// S3Config holds S3-compatible object storage settings (AWS S3, Cloudflare R2, MinIO, etc.).
type S3Config struct {
	Enabled        bool   `json:"enabled"`
	Endpoint       string `json:"endpoint"`
	Bucket         string `json:"bucket"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// FileStorage abstracts local disk and S3-compatible object storage.
type FileStorage interface {
	Save(ctx context.Context, key string, contentType string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
	Backend() string
}

// SwitchableStorage wraps a FileStorage so Settings can hot-swap between local disk and S3 at runtime.
type SwitchableStorage struct {
	mu      sync.RWMutex
	local   *LocalStorage
	current FileStorage
}

// NewSwitchableStorage initializes storage defaulting to localDir, switching to S3 if cfg.Enabled.
func NewSwitchableStorage(localDir string, cfg *S3Config) *SwitchableStorage {
	local := NewLocalStorage(localDir)
	sw := &SwitchableStorage{
		local:   local,
		current: local,
	}
	if cfg != nil && cfg.Enabled {
		if s3Client, err := NewS3Storage(*cfg); err == nil {
			sw.current = s3Client
		}
	}
	return sw
}

// Switch hot-swaps the active storage backend (testing S3 connectivity when enabled).
func (s *SwitchableStorage) Switch(ctx context.Context, cfg S3Config) (string, error) {
	if !cfg.Enabled {
		s.mu.Lock()
		s.current = s.local
		s.mu.Unlock()
		return s.local.Backend(), nil
	}

	s3Client, err := NewS3Storage(cfg)
	if err != nil {
		return "", err
	}
	if err := s3Client.TestConnection(ctx); err != nil {
		return "", err
	}

	s.mu.Lock()
	s.current = s3Client
	s.mu.Unlock()
	return s3Client.Backend(), nil
}

func (s *SwitchableStorage) active() FileStorage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *SwitchableStorage) Save(ctx context.Context, key, contentType string, data []byte) error {
	return s.active().Save(ctx, key, contentType, data)
}

func (s *SwitchableStorage) Open(ctx context.Context, key string) (io.ReadCloser, string, error) {
	return s.active().Open(ctx, key)
}

func (s *SwitchableStorage) Delete(ctx context.Context, key string) error {
	return s.active().Delete(ctx, key)
}

func (s *SwitchableStorage) Backend() string {
	return s.active().Backend()
}
