package migrations

import (
	"strings"
	"testing"
)

// TestAllReturnsEveryScriptInContiguousOrder guards the schema history: a
// misnamed script would be skipped silently by All, and a gap or duplicate
// version would make migration order ambiguous.
func TestAllReturnsEveryScriptInContiguousOrder(t *testing.T) {
	t.Parallel()

	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scripts := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			scripts++
		}
	}

	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != scripts {
		t.Fatalf("All returned %d migrations for %d embedded scripts; every script must be named NNN_name.sql", len(all), scripts)
	}
	for i, migration := range all {
		if want := i + 1; migration.Version != want {
			t.Fatalf("migration %d has version %d, want %d (versions must be contiguous from 1)", i, migration.Version, want)
		}
		if migration.Name == "" || strings.TrimSpace(migration.SQL) == "" {
			t.Fatalf("migration %d is missing a name or SQL body", migration.Version)
		}
	}
}

func TestParseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file        string
		wantVersion int
		wantName    string
		wantOK      bool
	}{
		{file: "001_initial.sql", wantVersion: 1, wantName: "initial", wantOK: true},
		{file: "030_authority_reconciliation_soak.sql", wantVersion: 30, wantName: "authority_reconciliation_soak", wantOK: true},
		{file: "initial.sql"},
		{file: "abc_initial.sql"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			version, name, ok := parseName(tt.file)
			if version != tt.wantVersion || name != tt.wantName || ok != tt.wantOK {
				t.Fatalf("parseName(%q) = %d, %q, %v; want %d, %q, %v", tt.file, version, name, ok, tt.wantVersion, tt.wantName, tt.wantOK)
			}
		})
	}
}
