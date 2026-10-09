package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func ownerHolds(context.Context, *sql.Tx, string) error { return nil }

var admissionNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func seedPendingItem(t *testing.T, tenant string) (*storage.DB, *PipelineStore) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	digest := make([]byte, 32)
	created, expires := kernel.FormatTime(admissionNow.Add(-time.Hour)), kernel.FormatTime(admissionNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at)
		VALUES ('trigger-1', ?, 'deployment', 'high', 'situation-1', 1, 10, 5, 'fast', 'admitted', X'5B5D', ?, ?)`, tenant, digest, created); err != nil {
		t.Fatalf("insert trigger evaluation: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO scheduler_items (scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, lane, priority, status, dedupe_key, expires_at, created_at, updated_at)
		VALUES ('item-1', 'trigger-1', ?, 'situation-1', 1, 'fast', 1, 'pending', ?, ?, ?, ?)`, tenant, digest, expires, created, created); err != nil {
		t.Fatalf("insert scheduler item: %v", err)
	}
	return db, &PipelineStore{DB: db, TenantID: tenant, RuntimeOwner: ownerHolds, OwnerEpoch: "epoch"}
}

func itemStatus(t *testing.T, db *storage.DB) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM scheduler_items WHERE scheduler_item_id = 'item-1'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestPollSchedulerQueueReadsOnlyTheStoresTenant(t *testing.T) {
	t.Parallel()
	_, own := seedPendingItem(t, "tenant")
	poll, err := own.PollSchedulerQueue(t.Context(), admissionNow)
	if err != nil || !poll.Found || poll.Next != "item-1" {
		t.Fatalf("own tenant poll = %+v, %v; want item-1", poll, err)
	}
	other := *own
	other.TenantID = "another-tenant"
	if poll, err := other.PollSchedulerQueue(t.Context(), admissionNow); err != nil || poll.Found {
		t.Fatalf("another tenant saw the queue: %+v, %v", poll, err)
	}
}

func TestPollSchedulerQueueNamesTheTenantWhenTheReadFails(t *testing.T) {
	t.Parallel()
	db, pipeline := seedPendingItem(t, "tenant")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.PollSchedulerQueue(t.Context(), admissionNow); err == nil {
		t.Fatal("polling a closed database succeeded")
	}
}

func TestAnAdmissionTransactionCommitsTogetherOrNotAtAll(t *testing.T) {
	t.Parallel()
	failure := errors.New("later step failed")
	tests := []struct {
		name       string
		after      error
		wantStatus string
	}{
		{"every step succeeded", nil, "coalesced"},
		{"a later step failed", failure, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, pipeline := seedPendingItem(t, "tenant")
			err := pipeline.InAdmission(t.Context(), func(tx *AdmissionTx) error {
				if err := tx.CoalesceSkipped(t.Context(), "item-1", admissionNow); err != nil {
					return err
				}
				return tt.after
			})
			if !errors.Is(err, tt.after) {
				t.Fatalf("InAdmission = %v, want %v", err, tt.after)
			}
			if status := itemStatus(t, db); status != tt.wantStatus {
				t.Fatalf("scheduler item status = %q, want %q", status, tt.wantStatus)
			}
		})
	}
}

func TestAdmissionStepsRecordWhyAnItemLeftTheQueue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		step       func(context.Context, *AdmissionTx) error
		wantStatus string
		wantReason string
	}{
		{"cost rejection", func(ctx context.Context, tx *AdmissionTx) error {
			if err := tx.RecordCostRejection(ctx, "item-1", errors.New("budget exhausted")); err != nil {
				return err
			}
			return tx.CoalesceCostRejected(ctx, "item-1", admissionNow)
		}, "coalesced", "budget exhausted"},
		{"expiry", func(ctx context.Context, tx *AdmissionTx) error {
			return tx.Expire(ctx, episodeledger.ExpiredItem{SchedulerItemID: "item-1", Reason: "expired before admission"}, admissionNow)
		}, "expired", "expired before admission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, pipeline := seedPendingItem(t, "tenant")
			if err := pipeline.InAdmission(t.Context(), func(tx *AdmissionTx) error { return tt.step(t.Context(), tx) }); err != nil {
				t.Fatal(err)
			}
			var reasons string
			if err := db.QueryRowContext(t.Context(), "SELECT CAST(reasons_json AS TEXT) FROM trigger_evaluations WHERE trigger_id = 'trigger-1'").Scan(&reasons); err != nil {
				t.Fatal(err)
			}
			if status := itemStatus(t, db); status != tt.wantStatus || !strings.Contains(reasons, tt.wantReason) {
				t.Fatalf("status %q, reasons %s; want %q recording %q", status, reasons, tt.wantStatus, tt.wantReason)
			}
		})
	}
}
