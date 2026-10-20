package app

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

const kindFileSuffix = ".kind.json"

var ErrNoKindFiles = errors.New("plantwire: the kind directory holds no *" + kindFileSuffix + " file")

func commissionedKinds(dir string) ([]domain.Kind, error) {
	paths, err := kindFiles(dir)
	if err != nil {
		return nil, err
	}
	return readKindFiles(paths)
}

func readKindFiles(paths []string) ([]domain.Kind, error) {
	kinds := make([]domain.Kind, 0, len(paths))
	for _, path := range paths {
		kind, err := readKindFile(path)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, kind)
	}
	return kinds, nil
}

func kindFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("plantwire: read the kind directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), kindFileSuffix) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoKindFiles, dir)
	}
	return paths, nil
}

func readKindFile(path string) (domain.Kind, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plantwire: read a kind file: %w", err)
	}
	kind, err := wire.ParseKindFile(raw)
	if err != nil {
		return nil, fmt.Errorf("plantwire: kind file %s: %w", path, err)
	}
	digest, err := domain.KindDigest(kind)
	if err != nil {
		return nil, fmt.Errorf("plantwire: kind file %s: %w", path, err)
	}
	slog.Info("plant wire kind file read", "kind", kind.Name(), "kind_digest", digest, "path", path)
	return kind, nil
}
