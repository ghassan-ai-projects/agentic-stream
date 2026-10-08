package storagetest

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Open opens the runtime database at path exactly as storage.Open does. When
// no database exists at path yet, it first seeds the file with the migrated
// template so the open finds every migration applied.
func Open(ctx context.Context, path string) (*storage.DB, error) {
	if err := seedFromTemplate(path); err != nil {
		return nil, err
	}
	db, err := storage.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open seeded database: %w", err)
	}
	return db, nil
}

func seedFromTemplate(path string) error {
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	template, err := migratedTemplate()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, template, 0o600); err != nil {
		return fmt.Errorf("seed database %s: %w", path, err)
	}
	return nil
}
