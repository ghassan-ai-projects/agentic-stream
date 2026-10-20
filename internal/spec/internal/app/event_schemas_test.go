package app_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
		{"a null document", schemaDirectory(t, map[string]string{"null.json": "null"}), "decode event schemas null.json: the document declares no event schema"},
		{"an empty file", schemaDirectory(t, map[string]string{"empty.json": ""}), "decode event schemas empty.json"},
		{"an upper-case extension", schemaDirectory(t, map[string]string{"fan.JSON": fanSpeedSchema}), "event schema file fan.JSON must end in .json"},
		{"a file larger than 1 MiB", schemaDirectory(t, map[string]string{"big.json": fanSpeedSchema + strings.Repeat(" ", 1<<20)}), "event schema file big.json is larger than 1048576 bytes"},
		{"more than 64 schema files", schemaDirectory(t, manySchemaFiles(65)), "holds 65 schema files, more than 64"},
		{"a symbolic link", linkedSchemaDirectory(t), "event schema file link.json is not a regular file"},
		{"a named pipe", pipeSchemaDirectory(t), "event schema file pipe.json is not a regular file"},
		{"a directory named like a schema file", nestedSchemaDirectory(t), "event schema file nested.json is not a regular file"},
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

func manySchemaFiles(count int) map[string]string {
	files := make(map[string]string, count)
	for i := range count {
		files[fmt.Sprintf("fan-%02d.json", i)] = fanSpeedSchema
	}
	return files
}

func linkedSchemaDirectory(t *testing.T) string {
	t.Helper()
	target := filepath.Join(schemaDirectory(t, map[string]string{"fan.json": fanSpeedSchema}), "fan.json")
	dir := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func pipeSchemaDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func nestedSchemaDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
