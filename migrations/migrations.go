// Package migrations provides the embedded SQLite migration scripts.
package migrations

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

// Migration is a numbered database migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// All returns all embedded migrations in version order.
func All() ([]Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, ok := parseName(entry.Name())
		if !ok {
			continue
		}
		data, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func parseName(name string) (int, string, bool) {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", false
	}
	return version, parts[1], true
}
