package storagetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

var migratedTemplate = sync.OnceValues(func() ([]byte, error) {
	return loadTemplate(filepath.Join(os.TempDir(), "agentic-stream-storagetest"))
})

func loadTemplate(directory string) ([]byte, error) {
	path, err := templatePath(directory)
	if err != nil {
		return nil, err
	}
	if content, err := readSoundTemplate(path); err == nil {
		return content, nil
	}
	if err := buildTemplate(path); err != nil {
		return nil, err
	}
	removeOtherTemplates(directory, path)
	return readSoundTemplate(path)
}

func templatePath(directory string) (string, error) {
	digest, err := migrationDigest()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "template-"+digest+".db"), nil
}

func migrationDigest() (string, error) {
	all, err := migrations.All()
	if err != nil {
		return "", fmt.Errorf("load migrations: %w", err)
	}
	hash := sha256.New()
	for _, m := range all {
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%s\x00", m.Version, m.Name, m.SQL)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

func buildTemplate(path string) error {
	scratch, err := newScratchDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	built := filepath.Join(scratch, "template.db")
	if err := migrateAndClose(built); err != nil {
		return err
	}
	if err := os.Rename(built, path); err != nil {
		return fmt.Errorf("publish template database: %w", err)
	}
	return nil
}

func newScratchDirectory(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create template directory: %w", err)
	}
	scratch, err := os.MkdirTemp(parent, "build-")
	if err != nil {
		return "", fmt.Errorf("create template scratch directory: %w", err)
	}
	return scratch, nil
}

func migrateAndClose(path string) error {
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		return fmt.Errorf("migrate template database: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close template database: %w", err)
	}
	return nil
}
