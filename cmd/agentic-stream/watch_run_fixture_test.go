package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const watchTrace = "../../examples/predictive-maintenance/testdata/trace-watch.jsonl"

var (
	sharedFixtureDir = sync.OnceValues(func() (string, error) { return os.MkdirTemp("", "as-fixtures") })
	watchRun         = sync.OnceValues(runWatchTrace)
)

func removeSharedFixtures() {
	if dir, err := sharedFixtureDir(); err == nil {
		_ = os.RemoveAll(dir)
	}
}

func watchRunDatabase(t *testing.T) string {
	t.Helper()
	path, err := watchRun()
	if err != nil {
		t.Fatalf("run the watch trace: %v", err)
	}
	return path
}

func runWatchTrace() (string, error) {
	dir, err := sharedFixtureDir()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	path := filepath.Join(dir, "watch-run.db")
	seeded, err := storagetest.Open(ctx, path)
	if err != nil {
		return "", err
	}
	if err := seeded.Close(); err != nil {
		return "", err
	}
	cmd := newRunLiveCommand()
	cmd.SetArgs([]string{"--db", path, "--spec", testSpec, "--trace", watchTrace})
	return path, cmd.ExecuteContext(ctx)
}
