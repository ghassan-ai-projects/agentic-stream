package app_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"testing"
	"time"
)

func TestEvaluationKeepsOriginalTransactionAndRollback(t *testing.T) {
	db, id := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	var original *sql.Tx
	checks := 0
	service := newTestService(t, func(c *policy.Config) {
		c.RuntimeOwner = func(_ context.Context, tx *sql.Tx, _ string) error {
			if tx != original {
				t.Fatal("owner transaction replaced")
			}
			checks++
			return nil
		}
		c.DecisionEpoch = func(_ context.Context, tx *sql.Tx, _ string) error {
			if tx != original {
				t.Fatal("epoch transaction replaced")
			}
			checks++
			return nil
		}
	})
	rollback := errors.New("caller rollback")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		original = tx
		r, err := service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: id, Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		if r.CommandID == "" {
			t.Fatal(r)
		}
		return rollback
	})
	if !errors.Is(err, rollback) || checks != 2 {
		t.Fatal(err, checks)
	}
	for _, table := range []string{"commands", "outbox", "policy_evaluations"} {
		var n int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT policy_status FROM intents WHERE intent_id=?", id).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
}
func TestEpochFailureCannotPrepareCommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
	}{{"unbound", control.ErrEpochUnbound, "epoch_unbound"}, {"killed", errors.New("killed"), "epoch_killed"}} {
		t.Run(tc.name, func(t *testing.T) {
			db, id := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
			defer func() { _ = db.Close() }()
			service := newTestService(t, func(c *policy.Config) {
				c.DecisionEpoch = func(context.Context, *sql.Tx, string) error { return tc.err }
			})
			if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				r, err := service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: id, Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
				if r.Result != "denied" || r.Reason != tc.reason || r.CommandID != "" {
					t.Fatal(r)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCalibrationIsReadFromTheOriginalTransactionWithExactBinding(t *testing.T) {
	for _, calibrated := range []bool{true, false} {
		t.Run(fmt.Sprint(calibrated), func(t *testing.T) {
			db, id := openPolicyFixture(t, "R2", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
			defer func() { _ = db.Close() }()
			if calibrated {
				activateCalibration(t, db, "test", "v1", "sha256:1111")
			}
			service := newTestService(t)
			if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				r, err := service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: id, Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
				if calibrated && (r.Result != "approved" || r.Reason != "calibrated_automation" || r.CommandID == "") {
					t.Fatal(r)
				}
				if !calibrated && (r.Result != "approval_required" || r.CommandID != "") {
					t.Fatal(r)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
