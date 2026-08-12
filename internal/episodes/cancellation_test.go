package episodes

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRunnerPersistsCancellationAfterExecutorCancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-cancel")

	runner := NewRunner(db, cancelingExecutor{cancel: cancel}, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}

	var status string
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM episode_attempts WHERE episode_id = 'epi-cancel'").Scan(&status); err != nil {
		t.Fatalf("read attempt status: %v", err)
	}
	if status != string(AttemptCancelled) {
		t.Fatalf("attempt status = %q, want %q", status, AttemptCancelled)
	}
	var reason string
	if err := db.QueryRowContext(context.Background(), "SELECT json_extract(terminal_json, '$.reason') FROM episode_attempts WHERE episode_id = 'epi-cancel'").Scan(&reason); err != nil {
		t.Fatalf("read cancellation reason: %v", err)
	}
	if reason != "worker_cancelled" { //nolint:misspell // Assert the frozen durable reason.
		t.Fatalf("cancellation reason = %q", reason)
	}
}

type cancelingExecutor struct{ cancel context.CancelFunc }

func (e cancelingExecutor) Execute(context.Context, *Request) (*Outcome, error) {
	e.cancel()
	return nil, context.Canceled
}

func (e cancelingExecutor) Name() string { return "canceling" }

var _ Executor = cancelingExecutor{}
