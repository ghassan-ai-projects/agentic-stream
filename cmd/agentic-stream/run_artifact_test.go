package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
)

func TestExportRunRejectsMissingPathsBeforeOpeningState(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"database", "output"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			dbPath := filepath.Join(t.TempDir(), "state.db")
			outputDir := filepath.Join(t.TempDir(), "artifact")
			dbArg, outputArg := dbPath, outputDir
			if missing == "database" {
				dbArg = ""
			} else {
				outputArg = ""
			}
			_, err := exportRunArtifact(t.Context(), dbArg, outputArg, runartifact.Manifest{})
			if err == nil || !strings.Contains(err.Error(), "--db and --output are required") {
				t.Fatalf("err=%v", err)
			}
			for _, path := range []string{dbPath, outputDir} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("validation created %s: err=%v", path, err)
				}
			}
		})
	}
}
