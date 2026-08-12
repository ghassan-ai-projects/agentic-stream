package episodes

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestRunnerCancelsSupersededStreamedAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "supersede.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-supersede")
	started := make(chan struct{})
	runner := NewRunner(db, blockingExecutor{started: started}, clock.Physical(), ids.Deterministic())
	result := make(chan error, 1)
	go func() { _, runErr := runner.RunOnce(ctx, "tenant"); result <- runErr }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'superseded' WHERE episode_id = ?", "epi-supersede"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("superseded executor was not canceled")
	}
	var status, lifecycle string
	if err := db.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE episode_id = 'epi-supersede'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-supersede'").Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if status != string(AttemptCancelled) || lifecycle != string(LifecycleSuperseded) {
		t.Fatalf("status=%q lifecycle=%q", status, lifecycle)
	}
}

type cancelingExecutor struct{ cancel context.CancelFunc }

func (e cancelingExecutor) Execute(context.Context, *Request) (*Outcome, error) {
	e.cancel()
	return nil, context.Canceled
}

func (e cancelingExecutor) Name() string { return "canceling" }

var _ Executor = cancelingExecutor{}

type blockingExecutor struct{ started chan<- struct{} }

func (e blockingExecutor) Execute(ctx context.Context, _ *Request) (*Outcome, error) {
	close(e.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (e blockingExecutor) Name() string { return "blocking" }
