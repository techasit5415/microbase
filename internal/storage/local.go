package storage

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
)

var ErrFileNotFound = errors.New("file not found")

// LocalStorage stores uploaded files in a directory on the local filesystem.
type LocalStorage struct {
	dir string
}

func NewLocalStorage(dir string) *LocalStorage {
	_ = os.MkdirAll(dir, 0o755)
	return &LocalStorage{dir: dir}
}

func (l *LocalStorage) Save(_ context.Context, key, _ string, data []byte) error {
	safeKey := filepath.Base(key)
	return os.WriteFile(filepath.Join(l.dir, safeKey), data, 0o644)
}

func (l *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	safeKey := filepath.Base(key)
	fullPath := filepath.Join(l.dir, safeKey)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrFileNotFound
		}
		return nil, "", err
	}
	ct := mime.TypeByExtension(filepath.Ext(safeKey))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return f, ct, nil
}

func (l *LocalStorage) Delete(_ context.Context, key string) error {
	safeKey := filepath.Base(key)
	err := os.Remove(filepath.Join(l.dir, safeKey))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (l *LocalStorage) Backend() string {
	return "local"
}
