package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

func LoadEventSchemas(dir string) (*domain.EventSchemas, error) {
	paths, err := eventSchemaFiles(dir)
	if err != nil {
		return nil, err
	}
	registry := domain.BuiltinEventSchemas()
	for _, path := range paths {
		if registry, err = mergeEventSchemaFile(registry, path); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func eventSchemaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read event schema directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("event schema directory %s holds no *.json file", dir)
	}
	return paths, nil
}

func mergeEventSchemaFile(registry *domain.EventSchemas, path string) (*domain.EventSchemas, error) {
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read event schema file: %w", err)
	}
	merged, err := registry.Merge(filepath.Base(path), document)
	if err != nil {
		return nil, fmt.Errorf("load event schemas: %w", err)
	}
	return merged, nil
}
