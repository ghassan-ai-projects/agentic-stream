package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRunnerQuarantinesAnEpisodeWhoseEpochIsAlreadyKilledBeforeAnyAttempt(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-killed")
	execWithoutForeignKeys(t, db, stmt("INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('epoch-fresh', 'killed', '2026-08-12T12:00:00.000000000Z')"))

	mustRunOnce(t, runnerWithEpochGate(db, fixture.New()))

	if got := lifecycleOf(t, db, "epi-killed"); got != string(episodeledger.LifecycleAbandoned) {
		t.Fatalf("lifecycle = %q, want abandoned", got)
	}
	if got := terminalReasonOf(t, db, "epi-killed"); got != "epoch_killed" {
		t.Fatalf("terminal reason = %q, want epoch_killed", got)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM episode_attempts WHERE episode_id = 'epi-killed'"); got != 0 {
		t.Fatalf("a killed epoch started %d attempts", got)
	}
}

func TestRunnerQuarantinesAnEpisodeWithoutAPolicyEpochBeforeAnyAttempt(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seed := newEpisodeSeed("epi-unbound")
	seed.PolicyEpoch = ""
	seed.insert(t, db)

	mustRunOnce(t, runnerWithEpochGate(db, fixture.New()))

	if lifecycle, reason := lifecycleOf(t, db, "epi-unbound"), terminalReasonOf(t, db, "epi-unbound"); lifecycle != string(episodeledger.LifecycleAbandoned) || reason != "epoch_unbound" {
		t.Fatalf("lifecycle=%q reason=%q, want abandoned and epoch_unbound", lifecycle, reason)
	}
}

func TestRunnerRefusesTheDecisionOfAnAttemptWhoseEpochWasKilledMidFlight(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-kill-late")
	started, release := make(chan struct{}), make(chan struct{})
	executor := executorFunc(lateProducer(started, release))
	result := startRunOnce(runnerWithEpochGate(db, executor))
	receive(t, started, "the executor to start")
	if err := (&control.EpochControl{DB: db}).Kill(t.Context(), "epoch-fresh"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}
	close(release)

	mustFinish(t, result, "the late outcome to be refused")
	if status, lifecycle := attemptStatusOf(t, db, "epi-kill-late"), lifecycleOf(t, db, "epi-kill-late"); status != string(episodeledger.AttemptAbandoned) || lifecycle != string(episodeledger.LifecycleAbandoned) {
		t.Fatalf("attempt status=%q lifecycle=%q, want abandoned and abandoned", status, lifecycle)
	}
	if got := terminalReasonOf(t, db, "epi-kill-late"); got != "epoch_killed_post_execute" {
		t.Fatalf("terminal reason = %q, want epoch_killed_post_execute", got)
	}
	if got := decisionCount(t, db, "epi-kill-late"); got != 0 {
		t.Fatalf("a killed epoch persisted %d decisions", got)
	}
}

func TestRunnerCancelsAnInFlightAttemptWhenItsEpochIsKilled(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-hostile")
	started := make(chan struct{})
	result := startRunOnce(runnerWithEpochGate(db, blockingExecutor{started: started}))
	receive(t, started, "the executor to start")

	if err := (&control.EpochControl{DB: db}).Kill(t.Context(), "epoch-fresh"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}

	mustFinish(t, result, "the in-flight attempt to be canceled")
	if got := attemptStatusOf(t, db, "epi-hostile"); got != string(episodeledger.AttemptCancelled) {
		t.Fatalf("in-flight attempt status = %q, want %q", got, episodeledger.AttemptCancelled)
	}
	if got := decisionCount(t, db, "epi-hostile"); got != 0 {
		t.Fatalf("a killed in-flight episode persisted %d decisions", got)
	}
}

func TestAPostExecuteKilledEpochEpisodeSettlesItsActualCost(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-kill-cost")
	reserveCost(t, db, "epi-kill-cost", 50)
	started, release := make(chan struct{}), make(chan struct{})
	produce := lateProducer(started, release)
	executor := executorFunc(func(ctx context.Context, req *app.Request) (*app.Outcome, error) {
		outcome, err := produce(ctx, req)
		if outcome != nil {
			outcome.CostMicrounits = 9
		}
		return outcome, err //nolint:wrapcheck // Test double.
	})
	result := startRunOnce(runnerWithEpochGate(db, executor).WithCostLedger(&control.CostLedger{}))
	receive(t, started, "the executor to start")
	if err := (&control.EpochControl{DB: db}).Kill(t.Context(), "epoch-fresh"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}
	close(release)
	mustFinish(t, result, "the late outcome to be refused")
	if status, actual := settledMicrounits(t, db, "epi-kill-cost"); status != "settled" || actual != 9 {
		t.Fatalf("reservation = %q with %d microunits, want the work actually done settled at 9", status, actual)
	}
}
