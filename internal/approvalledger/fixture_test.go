package approvalledger_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func inTx(t *testing.T, db *storage.DB, work func(ctx context.Context, tx *sql.Tx) error) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return work(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func approvalDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
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

func requestApproval(ctx context.Context, tx *sql.Tx, id, intentID, expiresAt string) error {
	return approvalledger.Request(ctx, tx, id, intentID, "t0", expiresAt, []byte(`{}`), "nonce-"+id)
}

func statusOf(t *testing.T, db *storage.DB, approvalID string) string {
	t.Helper()
	var state string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM approvals WHERE approval_id = ?", approvalID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func countNotifications(t *testing.T, db *storage.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

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
