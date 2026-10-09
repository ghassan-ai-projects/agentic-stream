package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var callUntil = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openLedgerDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	digest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_attempt_id, current_fence, accepted_at) VALUES ('episode-1', 'scheduler-1', 'tenant-1', 'situation-1', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'running', 'attempt-1', 1, '2026-08-12T12:00:00Z')`, digest, digest, []byte(`{}`)); err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES ('attempt-1', 'episode-1', 1, 'running', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	return db
}

func openStore(t *testing.T) (Store, *storage.DB) {
	t.Helper()
	db := openLedgerDB(t)
	return New(db, allowOwner, "epoch-1"), db
}

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func ledgerCall(callID string) domain.Call {
	return domain.Call{
		EpisodeID: "episode-1", CallID: callID, ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1",
		SituationVersion: 1, EntityID: "motor-1", Arguments: domain.EvidenceGetArguments{EntityID: "motor-1"},
		Deadline: callUntil.Add(time.Minute), AttemptID: "attempt-1", Fence: 1,
		Trace:   contractsv1.TraceContext{Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		MaxRows: 1, MaxBytes: 100, From: callUntil.Add(-time.Hour), Until: callUntil,
	}
}

func pendingReservation(call domain.Call) domain.Reservation {
	return domain.Reservation{
		Key:           reservationKey(call),
		RequestSHA256: make([]byte, 32), TokenID: "token", RuntimeEpoch: "epoch-1", Status: "running", Created: true,
	}
}

func reservationKey(call domain.Call) domain.ReservationKey {
	return domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
}

func inTx(t *testing.T, s Store, use func(*Tx) error) error {
	t.Helper()
	return s.WithTx(t.Context(), use)
}

func mustInTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := inTx(t, s, use); err != nil {
		t.Fatal(err)
	}
}

func reserve(t *testing.T, s Store, callID, owner string) domain.Reservation {
	t.Helper()
	call := ledgerCall(callID)
	pending := pendingReservation(call)
	mustInTx(t, s, func(tx *Tx) error {
		return tx.InsertReservation(t.Context(), call, pending, callUntil, time.Minute, owner)
	})
	return pending
}

func ledgerState(t *testing.T, db *storage.DB, callID string) (status, code string) {
	t.Helper()
	var errorCode sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT status, error_code FROM evidence_call_ledger WHERE call_id = ?`, callID).Scan(&status, &errorCode); err != nil {
		t.Fatalf("read ledger row %s: %v", callID, err)
	}
	return status, errorCode.String
}
