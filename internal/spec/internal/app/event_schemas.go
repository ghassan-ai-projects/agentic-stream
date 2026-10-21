package app

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

const (
	maxEventSchemaFiles     = 64
	maxEventSchemaFileBytes = 1 << 20
	eventSchemaExtension    = ".json"
)

func LoadEventSchemas(dir string) (*domain.EventSchemas, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("read event schema directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	return loadEventSchemaRoot(root, dir)
}

func loadEventSchemaRoot(root *os.Root, dir string) (*domain.EventSchemas, error) {
	names, err := eventSchemaFiles(root, dir)
	if err != nil {
		return nil, err
	}
	registry := domain.BuiltinEventSchemas()
	for _, name := range names {
		if registry, err = mergeEventSchemaFile(registry, root, name); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func eventSchemaFiles(root *os.Root, dir string) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("read event schema directory: %w", err)
	}
	var names []string
	for _, entry := range entries {
		named, err := eventSchemaFileName(entry)
		if err != nil {
			return nil, err
		}
		if named {
			names = append(names, entry.Name())
		}
	}
	return requireEventSchemaFileCount(names, dir)
}

func eventSchemaFileName(entry fs.DirEntry) (bool, error) {
	extension := filepath.Ext(entry.Name())
	if !strings.EqualFold(extension, eventSchemaExtension) {
		return false, nil
	}
	if extension != eventSchemaExtension {
		return false, fmt.Errorf("event schema file %s must end in %s", entry.Name(), eventSchemaExtension)
	}
	if !entry.Type().IsRegular() {
		return false, fmt.Errorf("event schema file %s is not a regular file (links, directories and devices are refused)", entry.Name())
	}
	return true, nil
}

func requireEventSchemaFileCount(names []string, dir string) ([]string, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("event schema directory %s holds no *%s file", dir, eventSchemaExtension)
	}
	if len(names) > maxEventSchemaFiles {
		return nil, fmt.Errorf("event schema directory %s holds %d schema files, more than %d", dir, len(names), maxEventSchemaFiles)
	}
	return names, nil
}

func mergeEventSchemaFile(registry *domain.EventSchemas, root *os.Root, name string) (*domain.EventSchemas, error) {
	document, err := readEventSchemaFile(root, name)
	if err != nil {
		return nil, err
	}
	merged, err := registry.Merge(name, document)
	if err != nil {
		return nil, fmt.Errorf("load event schemas: %w", err)
	}
	return merged, nil
}

func readEventSchemaFile(root *os.Root, name string) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open event schema file: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := requireBoundedRegularFile(file, name); err != nil {
		return nil, err
	}
	document, err := io.ReadAll(io.LimitReader(file, maxEventSchemaFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read event schema file %s: %w", name, err)
	}
	return document, requireEventSchemaSize(name, int64(len(document)))
}

func requireBoundedRegularFile(file *os.File, name string) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat event schema file %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("event schema file %s is not a regular file", name)
	}
	return requireEventSchemaSize(name, info.Size())
}

func requireEventSchemaSize(name string, size int64) error {
	if size > maxEventSchemaFileBytes {
		return fmt.Errorf("event schema file %s is larger than %d bytes", name, maxEventSchemaFileBytes)
	}
	return nil
}
