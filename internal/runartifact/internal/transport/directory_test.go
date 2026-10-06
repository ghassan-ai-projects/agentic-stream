package transport_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/transport"
)

func TestPublishWritesFilesAndChecksumsThenRefusesExistingOutput(t *testing.T) {
	t.Parallel()
	output, err := transport.ReserveOutput(filepath.Join(t.TempDir(), "artifact"))
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Publish(output, map[string][]byte{"a.json": []byte("{}\n")}); err != nil {
		t.Fatal(err)
	}
	entries, err := transport.Entries(output)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %v err = %v", entries, err)
	}
	if _, err := transport.ReserveOutput(output); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output = %v", err)
	}
}

func TestPublishRefusesPathEscapingNames(t *testing.T) {
	t.Parallel()
	output := filepath.Join(t.TempDir(), "artifact")
	if err := transport.Publish(output, map[string][]byte{"../x": nil}); err == nil {
		t.Fatal("escaping name accepted")
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatal("partial artifact published")
	}
}

func TestRequireDirectoryRefusesFiles(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transport.RequireDirectory(file); err == nil {
		t.Fatal("file accepted as directory")
	}
}
