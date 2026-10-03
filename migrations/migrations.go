// Package migrations provides the embedded SQLite migration scripts.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
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
	migrations, err := readMigrations(entries)
	if err != nil {
		return nil, err
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func readMigrations(entries []fs.DirEntry) ([]Migration, error) {
	var migrations []Migration
	for _, entry := range entries {
		migration, ok, err := readMigration(entry)
		if err != nil {
			return nil, err
		}
		if ok {
			migrations = append(migrations, migration)
		}
	}
	return migrations, nil
}

// readMigration loads one versioned .sql entry; other entries are skipped.
func readMigration(entry fs.DirEntry) (Migration, bool, error) {
	if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
		return Migration{}, false, nil
	}
	version, name, ok := parseName(entry.Name())
	if !ok {
		return Migration{}, false, nil
	}
	data, err := files.ReadFile(entry.Name())
	if err != nil {
		return Migration{}, false, fmt.Errorf("read migration %s: %w", entry.Name(), err)
	}
	return Migration{Version: version, Name: name, SQL: string(data)}, true, nil
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
