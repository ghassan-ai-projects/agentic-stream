package app_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/app"
)

const fanSpeedSchema = `{"fan.speed.observed/1.0": {"event_type": "fan.speed.observed", "schema_version": "1.0", "fields": {"rpm": {"path": "rpm", "unit": "rpm"}}}}`

func schemaDirectory(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadEventSchemasMergesEveryJSONFileOfTheDirectory(t *testing.T) {
	t.Parallel()
	dir := schemaDirectory(t, map[string]string{"fan.json": fanSpeedSchema, "again.json": fanSpeedSchema, "README.md": "not a schema"})
	registry, err := app.LoadEventSchemas(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup("fan.speed.observed/1.0"); !ok {
		t.Fatal("the directory schema is missing")
	}
	if _, ok := registry.Lookup("zone.temp.observed/1.0"); !ok {
		t.Fatal("the built-in schemas are missing")
	}
}

func TestLoadEventSchemasRefusesADirectoryItCannotUse(t *testing.T) {
	t.Parallel()
	conflicting := strings.Replace(fanSpeedSchema, `"unit": "rpm"`, `"unit": "hz"`, 1)
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"a missing directory", filepath.Join(t.TempDir(), "absent"), "read event schema directory"},
		{"a directory without schema files", schemaDirectory(t, map[string]string{"notes.txt": "x"}), "holds no *.json file"},
		{"a malformed file", schemaDirectory(t, map[string]string{"bad.json": "{"}), "decode event schemas bad.json"},
		{"two files that disagree on one ref", schemaDirectory(t, map[string]string{"a.json": fanSpeedSchema, "b.json": conflicting}), `b.json: event schema "fan.speed.observed/1.0" is already registered with a different definition`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := app.LoadEventSchemas(tt.dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadEventSchemas = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := app.LoadEventSchemas(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing directory = %v, want fs.ErrNotExist", err)
	}
}
