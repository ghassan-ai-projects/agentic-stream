package app_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestEvaluationRunsItsFencesOnTheCallersTransactionAndLeavesCommitToIt(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
	var original *sql.Tx
	checks := 0
	fenceOnOriginal := func(name string) func(context.Context, *sql.Tx, string) error {
		return func(_ context.Context, tx *sql.Tx, _ string) error {
			if tx != original {
				t.Errorf("%s fence ran on a transaction other than the caller's", name)
			}
			checks++
			return nil
		}
	}
	service := newTestService(t, func(c *policy.Config) {
		c.RuntimeOwner, c.DecisionEpoch = fenceOnOriginal("owner"), fenceOnOriginal("epoch")
	})
	rollback := errors.New("caller rollback")

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		original = tx
		result, err := service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: intentID, Now: fixtureNow})
		if err != nil || result.CommandID == "" {
			t.Errorf("evaluation = %+v, %v; want a prepared command", result, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) || checks != 2 {
		t.Fatalf("err = %v, fences run = %d; want the caller's rollback after 2 fences", err, checks)
	}
	for _, table := range []string{"commands", "outbox", "policy_evaluations"} {
		if rows := scalar[int](t, db, "SELECT COUNT(*) FROM "+table); rows != 0 {
			t.Errorf("%s kept %d rows after the caller rolled back", table, rows)
		}
	}
	if status := scalar[string](t, db, "SELECT policy_status FROM intents WHERE intent_id = ?", intentID); status != "pending" {
		t.Fatalf("intent status = %q, want pending after the caller rolled back", status)
	}
}

func TestADecisionEpochThatIsKilledOrUnboundDeniesTheIntentWithoutACommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{"unbound", control.ErrEpochUnbound, "epoch_unbound"},
		{"killed", errors.New("killed"), "epoch_killed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
			service := newTestService(t, func(c *policy.Config) {
				c.DecisionEpoch = func(context.Context, *sql.Tx, string) error { return test.err }
			})

			result := evaluateIntent(t, db, service, intentID, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
			if result.Result != "denied" || result.Reason != test.wantReason || result.CommandID != "" {
				t.Fatalf("result = %+v, want denied/%s without a command", result, test.wantReason)
			}
			if commands, outbox := commandAndOutboxCounts(t, db); commands != 0 || outbox != 0 {
				t.Fatalf("commands=%d outbox=%d after a refused epoch", commands, outbox)
			}
		})
	}
}
