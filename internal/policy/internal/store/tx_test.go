package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func TestFencesRunOnTheCallersOwnTransaction(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	refused := errors.New("fence refused")
	err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		fence := func(_ context.Context, got *sql.Tx, epoch string) error {
			if got != original || epoch != "epoch-1" {
				t.Errorf("fence ran on tx=%p epoch=%q, want the caller's %p and epoch-1", got, epoch, original)
			}
			return refused
		}
		return Join(original).Assert(t.Context(), fence, "epoch-1")
	})
	if !errors.Is(err, refused) {
		t.Fatalf("Assert = %v, want the fence's refusal", err)
	}
}

func TestTheActionInterlockIsReadOnTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		if err := tx.AssertInterlock(t.Context()); err != nil {
			t.Errorf("a ready interlock was refused: %v", err)
		}
		if _, err := interlock.TripIn(t.Context(), tx.tx, "operator stop", fixtureNow); err != nil {
			return err
		}
		if err := tx.AssertInterlock(t.Context()); !errors.Is(err, interlock.ErrTripped) {
			t.Errorf("a tripped interlock = %v, want %v", err, interlock.ErrTripped)
		}
		return nil
	})
}

func TestEveryJoinedWriteRollsBackWithTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	rollback := errors.New("caller rollback")
	err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		tx := Join(original)
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		row.RateLimitPerHour = 1
		for _, write := range []func() error{
			func() error { _, _, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow); return err },
			func() error { return tx.InsertCommandOutbox(t.Context(), command.ID, command.JSON, fixtureNow) },
			func() error { _, err := tx.DispatchWithinLimit(t.Context(), row, fixtureNow); return err },
			func() error {
				return tx.SetIntentStatus(t.Context(), domain.IntentStatusChange{IntentID: intentID, Status: "approved", Now: fixtureNow, Operation: ApproveIntentStatus})
			},
			func() error {
				return tx.RecordEvaluation(t.Context(), domain.EvaluationAudit{ID: "eval", PolicyVersion: "v1", PolicyDigest: "policy", Row: row, Result: domain.Result{Result: "approved", CommandID: command.ID}, Reason: "tested", Now: fixtureNow})
			},
			func() error {
				requestPendingApproval(t, tx, intentID, "approval-1", fixtureNow.Add(time.Hour), fixtureNow)
				return nil
			},
		} {
			if err := write(); err != nil {
				return err
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction = %v, want the caller's rollback", err)
	}
	for _, table := range []string{"commands", "outbox", "policy_evaluations", "intent_dispatch_counts", "approvals"} {
		if rows := scalar[int](t, db, "SELECT COUNT(*) FROM "+table); rows != 0 {
			t.Errorf("%s kept %d rows after the caller rolled back", table, rows)
		}
	}
	if status := scalar[string](t, db, "SELECT policy_status FROM intents WHERE intent_id = ?", intentID); status != "pending" {
		t.Errorf("intent status = %q, want pending", status)
	}
}

func TestAClosedTransactionFailsEveryOperationWithTheTransactionError(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow)
	original, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx, ctx, row := Join(original), t.Context(), domain.IntentRecord{IntentID: intentID}
	operations := map[string]func() error{
		"LoadIntent":             func() error { _, err := tx.LoadIntent(ctx, intentID); return err },
		"ExistingCommandID":      func() error { _, err := tx.ExistingCommandID(ctx, intentID); return err },
		"PendingApproval":        func() error { _, err := tx.PendingApproval(ctx, intentID); return err },
		"ApprovedApproval":       func() error { _, err := tx.ApprovedApproval(ctx, intentID); return err },
		"PendingApprovalExpiry":  func() error { _, _, err := tx.PendingApprovalExpiry(ctx, intentID); return err },
		"AssertionBinding":       func() error { _, _, err := tx.AssertionBinding(ctx, intentID); return err },
		"LoadApproval":           func() error { _, err := tx.LoadApproval(ctx, intentID, "tenant"); return err },
		"ApprovalSnapshotDigest": func() error { _, err := tx.ApprovalSnapshotDigest(ctx, row); return err },
		"ApprovalDelta":          func() error { _, err := tx.ApprovalDelta(ctx, intentID); return err },
		"ApprovalEntity":         func() error { _, err := tx.ApprovalEntity(ctx, "sit-policy"); return err },
		"RelayActivity":          func() error { _, err := tx.RelayActivity(ctx, "tenant", "relay-1"); return err },
		"ApproverKey":            func() error { _, err := tx.ApproverKey(ctx, "tenant", "operator-1"); return err },
		"ApprovalAuthority":      func() error { _, err := tx.ApprovalAuthority(ctx, row, "motor-1", "operator-1"); return err },
		"CompensationTenant":     func() error { _, _, err := tx.CompensationTenant(ctx, intentID); return err },
		"DispatchWithinLimit":    func() error { _, err := tx.DispatchWithinLimit(ctx, row, fixtureNow); return err },
		"SetIntentStatus": func() error {
			return tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: intentID, Status: "denied", Now: fixtureNow, Operation: SetPolicyStatus})
		},
		"RecordEvaluation":      func() error { return tx.RecordEvaluation(ctx, domain.EvaluationAudit{}) },
		"InsertCommand":         func() error { _, err := tx.InsertCommand(ctx, row, domain.CommandRecord{}, fixtureNow); return err },
		"InsertCommandOutbox":   func() error { return tx.InsertCommandOutbox(ctx, intentID, nil, fixtureNow) },
		"RemovePreparedCommand": func() error { return tx.RemovePreparedCommand(ctx, intentID, intentID) },
		"GovernanceSummary":     func() error { _, err := tx.GovernanceSummary(ctx, "tenant"); return err },
		"ReplaceGovernance": func() error {
			_, err := tx.ReplaceGovernance(ctx, domain.PrincipalDocument{Tenant: "tenant"}, fixtureNow)
			return err
		},
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("%s on a closed transaction = %v, want %v", name, err, sql.ErrTxDone)
		}
	}
}
