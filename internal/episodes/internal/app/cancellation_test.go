package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const frozenCancellationReason = "worker_cancelled" //nolint:misspell // The durable protocol reason is frozen.

func TestRunnerPersistsCancellationAfterExecutorCancelsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-cancel")
	executor := executorFunc(func(context.Context, *app.Request) (*app.Outcome, error) {
		cancel()
		return nil, context.Canceled
	})

	if ran, err := permissiveRunner(db, executor).RunOnce(ctx, episodeTenant); err != nil || !ran {
		t.Fatalf("RunOnce ran=%v err=%v, want true nil", ran, err)
	}

	if got := attemptStatusOf(t, db, "epi-cancel"); got != string(episodeledger.AttemptCancelled) {
		t.Fatalf("attempt status = %q, want %q", got, episodeledger.AttemptCancelled)
	}
	if got := scalar[string](t, db, "SELECT json_extract(terminal_json, '$.reason') FROM episode_attempts WHERE episode_id = 'epi-cancel'"); got != frozenCancellationReason {
		t.Fatalf("cancellation reason = %q, want %q", got, frozenCancellationReason)
	}
}

func TestRunnerPersistsADeclinedOutcomeAfterTheParentContextIsCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-cancel-declined")
	executor := executorFunc(func(_ context.Context, req *app.Request) (*app.Outcome, error) {
		cancel()
		return &app.Outcome{Status: string(episodeledger.AttemptDeclined), AttemptID: req.AttemptID, Fence: req.Fence}, nil
	})

	if ran, err := permissiveRunner(db, executor).RunOnce(ctx, episodeTenant); err != nil || !ran {
		t.Fatalf("RunOnce ran=%v err=%v, want true nil", ran, err)
	}

	if status, lifecycle := attemptStatusOf(t, db, "epi-cancel-declined"), lifecycleOf(t, db, "epi-cancel-declined"); status != string(episodeledger.AttemptDeclined) || lifecycle != string(episodeledger.LifecycleConcluded) {
		t.Fatalf("attempt status=%q lifecycle=%q, want declined and concluded", status, lifecycle)
	}
	if got := decisionCount(t, db, "epi-cancel-declined"); got != 0 {
		t.Fatalf("declined outcome created %d decisions", got)
	}
}

func TestRunnerPersistsAProducedOutcomeAfterTheParentContextIsCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-cancel-produced")
	executor := executorFunc(func(_ context.Context, req *app.Request) (*app.Outcome, error) {
		cancel()
		return fixture.New().Execute(context.WithoutCancel(ctx), req)
	})

	if ran, err := permissiveRunner(db, executor).RunOnce(ctx, episodeTenant); err != nil || !ran {
		t.Fatalf("RunOnce ran=%v err=%v, want true nil", ran, err)
	}

	if status, lifecycle := attemptStatusOf(t, db, "epi-cancel-produced"), lifecycleOf(t, db, "epi-cancel-produced"); status != string(episodeledger.AttemptProduced) || lifecycle != string(episodeledger.LifecycleConcluded) {
		t.Fatalf("attempt status=%q lifecycle=%q, want produced and concluded", status, lifecycle)
	}
	if got := scalar[string](t, db, "SELECT validation_status FROM decisions WHERE episode_id = 'epi-cancel-produced'"); got != "accepted" {
		t.Fatalf("decision validation = %q, want accepted", got)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM intents WHERE decision_id = (SELECT decision_id FROM decisions WHERE episode_id = 'epi-cancel-produced')"); got != 1 {
		t.Fatalf("persisted intents = %d, want 1", got)
	}
}

func TestRunnerCancelsTheStreamedAttemptOfASupersededEpisode(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-supersede")
	started := make(chan struct{})
	result := startRunOnce(permissiveRunner(db, blockingExecutor{started: started}))
	receive(t, started, "the executor to start")

	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = 'superseded' WHERE episode_id = 'epi-supersede'"); err != nil {
		t.Fatal(err)
	}

	mustFinish(t, result, "the superseded executor to be canceled")
	if status, lifecycle := attemptStatusOf(t, db, "epi-supersede"), lifecycleOf(t, db, "epi-supersede"); status != string(episodeledger.AttemptCancelled) || lifecycle != string(episodeledger.LifecycleSuperseded) {
		t.Fatalf("attempt status=%q lifecycle=%q, want canceled and superseded", status, lifecycle)
	}
	if got := decisionCount(t, db, "epi-supersede"); got != 0 {
		t.Fatalf("a superseded episode persisted %d decisions", got)
	}
}
