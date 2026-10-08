package approvalledger_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func approvalDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	// Policy acceptance owns signature and provenance validation; isolate state and notification atomicity here.

	if _, err := db.ExecContext(t.Context(), `INSERT INTO decisions (decision_id,episode_id,attempt_id,fence,ordinal,situation_id,situation_version,raw_json,decision_sha256,validation_status,validation_json,created_at)
 VALUES ('decision','episode','attempt',1,1,'situation',1,X'7B7D',?,'accepted',X'7B7D','now')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id      string
		version int
	}{{"old", 1}, {"current", 2}, {"expired", 1}, {"denied", 1}} {
		insertIntent(t, db, item.id, item.version)
	}
	return db
}

func insertIntent(t *testing.T, db *storage.DB, id string, version int) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `INSERT INTO intents (intent_id,decision_id,tenant_id,situation_id,situation_version,intent_type,risk_class,intent_json,intent_sha256,expires_at,policy_status,created_at,updated_at)
 VALUES (?,'decision','tenant','situation',?,'review','R2',X'7B7D',?,'later','approval_required','now','now')`, id, version, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalTransitionsPreserveTerminalStateAndAssertionBinding(t *testing.T) {
	db := approvalDB(t)
	ctx := t.Context()
	digest := make([]byte, 32)
	digest[0] = 7
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, id := range []string{"old", "current", "expired", "denied"} {
			if err := approvalledger.Request(ctx, tx, id, id, "now", "later", []byte(`{}`), "nonce-"+id); err != nil {
				return err
			}
		}
		if err := approvalledger.BindAssertion(ctx, tx, "old", digest); err != nil {
			return err
		}
		if err := approvalledger.Resolve(ctx, tx, "old", "approved", "human", "relay", "allowed", "decided"); err != nil {
			return err
		}
		if err := approvalledger.Withdraw(ctx, tx, "old", "later"); err != nil {
			return err
		}
		if err := approvalledger.Expire(ctx, tx, "expired", "decided"); err != nil {
			return err
		}
		if err := approvalledger.ExpireIntent(ctx, tx, "current"); err != nil {
			return err
		}
		return approvalledger.Withdraw(ctx, tx, "denied", "decided")
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ id, status string }{{"old", "approved"}, {"current", "expired"}, {"expired", "expired"}, {"denied", "denied"}} {
		var state string
		if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id=?", want.id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != want.status {
			t.Fatalf("%s status=%s want=%s", want.id, state, want.status)
		}
	}
	var nonce, approver, relay string
	var bound []byte
	if err := db.QueryRowContext(ctx, "SELECT nonce,assertion_sha256,approver_identity,relay_identity FROM approvals WHERE approval_id='old'").Scan(&nonce, &bound, &approver, &relay); err != nil {
		t.Fatal(err)
	}
	if nonce != "nonce-old" || len(bound) != 32 || bound[0] != 7 || approver != "human" || relay != "relay" {
		t.Fatalf("assertion/principal binding changed: nonce=%s digest=%x human=%s relay=%s", nonce, bound, approver, relay)
	}
}

func TestSupersededWithdrawalAndNotificationShareTransaction(t *testing.T) {
	db := approvalDB(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	clk := sources.NewVirtual(now)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, id := range []string{"old", "current"} {
			if err := approvalledger.Request(ctx, tx, id, id, "now", "later", []byte(`{}`), "nonce-"+id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("downstream participant failed")
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := approvalledger.WithdrawSuperseded(ctx, tx, "situation", 2, "withdrawn", withdrawalPublisher("tenant", clk)); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("withdrawal rollback: %v", err)
	}
	var state string
	var count int
	if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id='old'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatalf("withdrawal escaped rollback: %s", state)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("withdrawal notification escaped rollback")
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return approvalledger.WithdrawSuperseded(ctx, tx, "situation", 2, "withdrawn", withdrawalPublisher("tenant", clk))
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ id, status string }{{"old", "denied"}, {"current", "pending"}} {
		if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id=?", want.id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != want.status {
			t.Fatalf("%s status=%s want=%s", want.id, state, want.status)
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("withdrawal notifications=%d", count)
	}
}

// withdrawalPublisher publishes the approval.withdrawn notification the way the
// cognition store does, reading the clock after each withdrawal.
func withdrawalPublisher(tenantID string, clk sources.Clock) approvalledger.WithdrawalPublisher {
	return func(ctx context.Context, tx *sql.Tx, w approvalledger.Withdrawal) error {
		err := notify.AppendLifecycleEvent(ctx, tx, notify.LifecycleEvent{
			ID: "approval.withdrawn:" + w.ApprovalID, TenantID: tenantID, Subject: "approval/" + w.ApprovalID, PartitionKey: w.SituationID,
			Payload: notify.ApprovalWithdrawn{ApprovalID: w.ApprovalID, IntentID: w.IntentID, SituationID: w.SituationID, SituationVersion: w.SituationVersion, Reason: "situation_version_conflict"},
			At:      clk.Now().UTC(), Trace: contractsv1.TraceContext{Traceparent: w.Traceparent, Tracestate: w.Tracestate},
		})
		if err != nil {
			return fmt.Errorf("append superseded approval notification: %w", err)
		}
		return nil
	}
}

func TestWithdrawingSupersededApprovalsRequiresAPublisher(t *testing.T) {
	db := approvalDB(t)
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return approvalledger.WithdrawSuperseded(t.Context(), tx, "situation", 2, "withdrawn", nil)
	})
	if !errors.Is(err, approvalledger.ErrPublisherRequired) {
		t.Fatalf("silent withdrawal accepted: %v", err)
	}
}
