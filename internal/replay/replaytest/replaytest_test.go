package replaytest_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/replaytest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestWithDatabaseOpenerRoutesEveryReplayThroughTheOpener(t *testing.T) {
	t.Parallel()
	opened := ""
	failure := errors.New("injected opener failure")
	ctx := replaytest.WithDatabaseOpener(t.Context(), func(_ context.Context, path string) (*storage.DB, error) {
		opened = path
		return nil, failure
	})
	request := replay.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: "unused", TracePath: "unused", TenantID: "default"}
	if _, err := replay.Run(ctx, request); !errors.Is(err, failure) {
		t.Fatalf("replay.Run = %v, want the opener's error", err)
	}
	if opened != request.DBPath {
		t.Fatalf("opener received %q, want %q", opened, request.DBPath)
	}
}
