package app_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

func TestCompileFileReadsAndSealsAnExampleSpec(t *testing.T) {
	t.Parallel()
	compiled, err := app.CompileFile(t.Context(), filepath.Join("..", "..", "..", "..", "examples", "predictive-maintenance", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(compiled.Digest, "sha256:") || len(compiled.CanonicalJSON) == 0 || compiled.Metadata.Name != "motor_bearing_degradation" {
		t.Fatalf("spec was not sealed: digest %q, name %q", compiled.Digest, compiled.Metadata.Name)
	}
}

func TestCompileFileReportsAnUnreadableFile(t *testing.T) {
	t.Parallel()
	_, err := app.CompileFile(t.Context(), filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read file") {
		t.Fatalf("err = %v, want fs.ErrNotExist wrapped by read file", err)
	}
}

func TestCompileFileReturnsTheCompilerDiagnosticUnchanged(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "wrong-kind.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: agentic-stream/v1\nkind: Other\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := app.CompileFile(t.Context(), path)

	var compileErr *domain.CompileError
	if !errors.As(err, &compileErr) || compileErr.Path != "kind" {
		t.Fatalf("err = %v, want a *domain.CompileError at kind", err)
	}
}
