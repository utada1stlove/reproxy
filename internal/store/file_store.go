package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/utada1stlove/reproxy/internal/app"
)

type FileStore struct {
	path string
}

type persistedRoutes struct {
	Routes []app.Route `json:"routes"`
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load(ctx context.Context) ([]app.Route, error) {
	_ = ctx

	content, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []app.Route{}, nil
	}

	if err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(content)) == 0 {
		return []app.Route{}, nil
	}

	var payload persistedRoutes
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}

	if payload.Routes == nil {
		return []app.Route{}, nil
	}

	return payload.Routes, nil
}

func (s *FileStore) Save(ctx context.Context, routes []app.Route) error {
	_ = ctx

	content, err := json.MarshalIndent(persistedRoutes{Routes: routes}, "", "  ")
	if err != nil {
		return err
	}

	content = append(content, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, "routes-*.json")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, s.path)
}
