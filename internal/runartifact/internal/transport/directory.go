package transport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
)

// ReserveOutput resolves path to an absolute directory that does not exist yet.
func ReserveOutput(path string) (string, error) {
	output, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve run artifact output: %w", err)
	}
	info, err := os.Stat(output)
	if err == nil {
		return "", existingOutputError(info, output)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect run artifact output: %w", err)
	}
	return output, nil
}

func existingOutputError(info os.FileInfo, output string) error {
	if !info.IsDir() {
		return fmt.Errorf("run artifact output is not a directory: %s", output)
	}
	return fmt.Errorf("run artifact output already exists; choose a new directory: %s", output)
}

// Publish writes the files and their checksum index into a private temporary
// directory beside output, then renames it into place, so a reader never sees a
// partial artifact.
func Publish(output string, files map[string][]byte) error {
	tmp, err := os.MkdirTemp(filepath.Dir(output), ".run-artifact-")
	if err != nil {
		return fmt.Errorf("create run artifact temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // directories need owner-only execute permission
		return fmt.Errorf("protect run artifact temporary directory: %w", err)
	}
	if err := writeContents(tmp, files); err != nil {
		return err
	}
	if err := os.Rename(tmp, output); err != nil {
		return fmt.Errorf("publish run artifact: %w", err)
	}
	return nil
}

func writeContents(dir string, files map[string][]byte) error {
	for name, data := range files {
		if err := writeFile(dir, name, data); err != nil {
			return err
		}
	}
	return writeFile(dir, domain.ChecksumsFile, domain.ChecksumIndex(files))
}

func writeFile(dir, name string, data []byte) error {
	if err := domain.RequireFileName(name); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// Read returns one file of an artifact directory.
func Read(dir, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name)) //nolint:wrapcheck // callers add the operation-specific file context.
}

// RequireDirectory refuses a path that is not an existing directory.
func RequireDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat run artifact: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("run artifact is not a directory")
	}
	return nil
}

// Entries lists the names in an artifact directory.
func Entries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read run artifact directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}
