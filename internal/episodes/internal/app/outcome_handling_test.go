package app_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func rejectionReasonsOf(t *testing.T, db *storage.DB, episodeID string) string {
	t.Helper()
	return scalar[string](t, db, "SELECT COALESCE(group_concat(reason), '') FROM episode_rejections WHERE episode_id = ?", episodeID)
}

func TestRunnerFailsAnAttemptWhoseExecutorReturnsNoOutcome(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-nil")
	executor := executorFunc(func(context.Context, *app.Request) (*app.Outcome, error) { return nil, nil }) //nolint:nilnil // The executor contract violation under test.

	mustRunOnce(t, permissiveRunner(db, executor))

	if got := scalar[string](t, db, "SELECT json_extract(terminal_json, '$.reason') FROM episode_attempts WHERE episode_id = 'epi-nil'"); got != "executor_returned_nil_outcome" {
		t.Fatalf("attempt reason = %q, want executor_returned_nil_outcome", got)
	}
	if got := attemptStatusOf(t, db, "epi-nil"); got != "failed" || lifecycleOf(t, db, "epi-nil") != "running" {
		t.Fatalf("attempt %q lifecycle %q, want a failed attempt and an episode kept running for retry", got, lifecycleOf(t, db, "epi-nil"))
	}
}

func TestRunnerRejectsAnOutcomeThatDoesNotCarryTheAttemptIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		identity   func(req *app.Request) (string, int64)
		wantReason episodeledger.RejectionReason
	}{
		{"another attempt at the current fence", func(req *app.Request) (string, int64) { return "att-foreign", req.Fence }, episodeledger.RejectWrongAttempt},
		{"an older fence", func(req *app.Request) (string, int64) { return req.AttemptID, req.Fence - 1 }, episodeledger.RejectStaleAttempt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			seedEpisode(t, db, "epi-identity")
			executor := executorFunc(func(ctx context.Context, req *app.Request) (*app.Outcome, error) {
				outcome, err := fixture.New().Execute(ctx, req)
				if err != nil {
					return nil, err //nolint:wrapcheck // Test double.
				}
				outcome.AttemptID, outcome.Fence = tc.identity(req)
				return outcome, nil
			})

			mustRunOnce(t, permissiveRunner(db, executor))

			if got := rejectionReasonsOf(t, db, "epi-identity"); got != string(tc.wantReason) {
				t.Fatalf("recorded rejections = %q, want %q", got, tc.wantReason)
			}
			if got := attemptStatusOf(t, db, "epi-identity"); got != "failed" {
				t.Fatalf("attempt status = %q, want failed", got)
			}
			if got := decisionCount(t, db, "epi-identity"); got != 0 {
				t.Fatalf("an outcome with a foreign identity persisted %d decisions", got)
			}
		})
	}
}

func TestRunnerRecordsADecisionTheContractRejectsAndNeverGovernsIt(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-invalid")
	executor := executorFunc(func(_ context.Context, req *app.Request) (*app.Outcome, error) {
		return &app.Outcome{Status: string(episodeledger.AttemptProduced), AttemptID: req.AttemptID, Fence: req.Fence, DecisionJSON: []byte(`{"decision_id":"dec-invalid"}`)}, nil
	})

	mustRunOnce(t, permissiveRunner(db, executor))

	var status, reason string
	if err := db.QueryRowContext(t.Context(), "SELECT validation_status, rejection_reason FROM decisions WHERE episode_id = 'epi-invalid'").Scan(&status, &reason); err != nil {
		t.Fatalf("the rejected decision must stay on record: %v", err)
	}
	if status != "rejected" || reason != "schema_invalid" {
		t.Fatalf("decision = %q (%q), want rejected for schema_invalid", status, reason)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM intents"); got != 0 {
		t.Fatalf("a rejected decision created %d intents", got)
	}
	if got := rejectionReasonsOf(t, db, "epi-invalid"); got != "schema_invalid" {
		t.Fatalf("recorded rejections = %q, want schema_invalid", got)
	}
	if got := attemptStatusOf(t, db, "epi-invalid"); got != "failed" {
		t.Fatalf("attempt status = %q, want failed", got)
	}
}

func TestRunnerRefusesToFinishAnAttemptWithANonTerminalStatus(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-open")
	executor := executorFunc(func(_ context.Context, req *app.Request) (*app.Outcome, error) {
		return &app.Outcome{Status: string(episodeledger.AttemptRunning), AttemptID: req.AttemptID, Fence: req.Fence}, nil
	})

	_, err := permissiveRunner(db, executor).RunOnce(t.Context(), episodeTenant)

	if err == nil || !strings.Contains(err.Error(), `non-terminal attempt status "running"`) {
		t.Fatalf("RunOnce error = %v, want a non-terminal attempt status refusal", err)
	}
	if got := lifecycleOf(t, db, "epi-open"); got == "concluded" {
		t.Fatalf("lifecycle = %q, a refused outcome must not conclude the episode", got)
	}
}

func reserveCost(t *testing.T, db *storage.DB, episodeID string, amount uint64) {
	t.Helper()
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return (&control.CostLedger{}).Reserve(t.Context(), tx, episodeID, episodeTenant, amount, "2026-08-12T10:00:00Z")
	})
	if err != nil {
		t.Fatalf("reserve cost: %v", err)
	}
}

func settledMicrounits(t *testing.T, db *storage.DB, episodeID string) (string, int) {
	t.Helper()
	var status string
	var actual int
	if err := db.QueryRowContext(t.Context(), "SELECT status, actual_micro FROM cost_reservations WHERE episode_id = ?", episodeID).Scan(&status, &actual); err != nil {
		t.Fatalf("read cost reservation: %v", err)
	}
	return status, actual
}

func TestRunnerSettlesTheReservedCostWithWhatTheWorkerReported(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-cost")
	reserveCost(t, db, "epi-cost", 50)
	executor := executorFunc(func(ctx context.Context, req *app.Request) (*app.Outcome, error) {
		outcome, err := fixture.New().Execute(ctx, req)
		if outcome != nil {
			outcome.CostMicrounits = 7
		}
		return outcome, err //nolint:wrapcheck // Test double.
	})

	mustRunOnce(t, permissiveRunner(db, executor).WithCostLedger(&control.CostLedger{}))

	if status, actual := settledMicrounits(t, db, "epi-cost"); status != "settled" || actual != 7 {
		t.Fatalf("reservation = %q with %d microunits, want settled with 7", status, actual)
	}
}

func TestRunnerReleasesTheReservedCostOfAnEpisodeItGivesUp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		killEpoch bool
		runs      int
	}{
		{"after three failed attempts", false, 3},
		{"when its policy epoch is killed before dispatch", true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			seedEpisode(t, db, "epi-released")
			reserveCost(t, db, "epi-released", 50)
			if tc.killEpoch {
				execWithoutForeignKeys(t, db, stmt("INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('epoch-fresh', 'killed', '2026-08-12T12:00:00.000000000Z')"))
			}
			runner := runnerWithEpochGate(db, &failingExecutor{failures: 10}).WithCostLedger(&control.CostLedger{})

			for range tc.runs {
				mustRunOnce(t, runner)
			}

			if status, actual := settledMicrounits(t, db, "epi-released"); status != "settled" || actual != 0 {
				t.Fatalf("reservation = %q with %d microunits, want settled with 0", status, actual)
			}
		})
	}
}

func TestRunnerConcludesAnEpisodeAfterThreeRejectedOutcomes(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-forged")
	reserveCost(t, db, "epi-forged", 50)
	executor := executorFunc(func(ctx context.Context, req *app.Request) (*app.Outcome, error) {
		outcome, err := fixture.New().Execute(ctx, req)
		if outcome != nil {
			outcome.AttemptID = "att-foreign"
		}
		return outcome, err //nolint:wrapcheck // Test double.
	})
	runner := permissiveRunner(db, executor).WithCostLedger(&control.CostLedger{})

	for range 3 {
		mustRunOnce(t, runner)
	}

	if lifecycle, reason := lifecycleOf(t, db, "epi-forged"), terminalReasonOf(t, db, "epi-forged"); lifecycle != "concluded" || reason != "worker_identity_mismatch" {
		t.Fatalf("lifecycle=%q reason=%q, want concluded for worker_identity_mismatch", lifecycle, reason)
	}
	if status, actual := settledMicrounits(t, db, "epi-forged"); status != "settled" || actual != 0 {
		t.Fatalf("reservation = %q with %d microunits, want settled with 0", status, actual)
	}
	if got := decisionCount(t, db, "epi-forged"); got != 0 {
		t.Fatalf("forged outcomes persisted %d decisions", got)
	}
}
