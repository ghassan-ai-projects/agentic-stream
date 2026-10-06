package app_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/app"
)

func TestCompileFileReadsAndSealsAnExampleSpec(t *testing.T) {
	t.Parallel()
	compiled, err := app.CompileFile(t.Context(), filepath.Join("..", "..", "..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Digest == "" || len(compiled.CanonicalJSON) == 0 || compiled.Metadata.Name == "" {
		t.Fatalf("spec was not sealed: %+v", compiled.Metadata)
	}
}

func TestCompileFileReportsAnUnreadableFile(t *testing.T) {
	t.Parallel()
	if _, err := app.CompileFile(t.Context(), filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing file compiled")
	}
}
