package evidence

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestLedgerReservesAndReplaysCompletedCall(t *testing.T) {
	db := openLedgerDB(t)
	call := ledgerTestCall()
	ledger := &Ledger{DB: db, LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Lease: time.Minute, Now: func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }}
	first, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1")
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if first.Completed != nil || first.Status != "running" {
		t.Fatalf("first reservation = %+v", first)
	}
	result := QueryResult{JSON: []byte(`{"rows":[{"value":42}]}`), RowCount: 1}
	if err := ledger.Complete(t.Context(), first, result); err != nil {
		t.Fatalf("complete: %v", err)
	}
	second, err := ledger.Reserve(t.Context(), call, "token-1", "epoch-1")
	if err != nil {
		t.Fatalf("replay reserve: %v", err)
	}
	if second.Completed == nil || string(second.Completed.JSON) != string(result.JSON) || second.Completed.RowCount != 1 {
		t.Fatalf("replayed result = %+v", second)
	}

	changed := call
	changed.Arguments = EvidenceGetArguments{EntityID: "other"}
	if _, err := ledger.Reserve(t.Context(), changed, "token-1", "epoch-1"); err == nil {
		t.Fatal("changed request was accepted")
	}
}

func TestLedgerConcurrentReservationHasOneWinner(t *testing.T) {
	db := openLedgerDB(t)
	call := ledgerTestCall()
	var winners atomic.Int32
	results := make(chan error, 2)
	for range 2 {
		go func() {
			ledger := &Ledger{DB: db, LeaseOwner: "owner", RuntimeEpoch: "epoch", Lease: time.Minute}
			reservation, err := ledger.Reserve(context.Background(), call, "token", "epoch")
			if err == nil && reservation.Created {
				winners.Add(1)
			}
			results <- err
		}()
	}
	for range 2 {
		<-results
	}
	if winners.Load() != 1 {
		t.Fatalf("reservation winners = %d, want 1", winners.Load())
	}
}

func TestLedgerRecoverTxInterruptsPriorEpochOnly(t *testing.T) {
	db := openLedgerDB(t)
	legacyHash := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO evidence_call_ledger (
			tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch,
			tool_name, situation_id, situation_version, entity_id, time_from, time_until,
			max_rows, max_bytes, deadline, request_sha256, status, lease_owner,
			lease_until, reserved_at
		) VALUES ('tenant-1', 'episode-1', 'attempt-1', 1, 'call-old', 'token-old', 'epoch-old',
			'evidence.get', 'situation-1', 1, 'motor-1', '2026-08-12T11:00:00Z',
			'2026-08-12T12:00:00Z', 1, 100, '2026-08-12T12:01:00Z', ?, 'running',
			'owner', '2026-08-12T12:10:00Z', '2026-08-12T12:00:00Z')`, legacyHash); err != nil {
		t.Fatalf("insert old evidence call: %v", err)
	}
	ledger := &Ledger{DB: db, LeaseOwner: "owner", RuntimeEpoch: "epoch-new"}
	now := time.Date(2026, 8, 12, 12, 2, 0, 0, time.UTC)
	var recovered int
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		recovered, err = ledger.RecoverTx(t.Context(), tx, now)
		return err
	}); err != nil {
		t.Fatalf("recover evidence calls: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered calls = %d, want 1", recovered)
	}
	var status, code string
	if err := db.QueryRowContext(t.Context(), `SELECT status, error_code FROM evidence_call_ledger WHERE call_id = 'call-old'`).Scan(&status, &code); err != nil {
		t.Fatalf("read recovered call: %v", err)
	}
	if status != "interrupted" || code != "runtime_restart" {
		t.Fatalf("recovered call status=%q code=%q", status, code)
	}
	tx := mustBeginTx(t, db)
	if recovered, err := ledger.RecoverTx(t.Context(), tx, now); err != nil || recovered != 0 {
		t.Fatalf("repeat recovery = %d, err=%v; want zero", recovered, err)
	}
}

func mustBeginTx(t *testing.T, db *storage.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func openLedgerDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(t.Context(), filepath.Join(dir, "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_attempt_id, current_fence, accepted_at) VALUES ('episode-1', 'scheduler-1', 'tenant-1', 'situation-1', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'running', 'attempt-1', 1, '2026-08-12T12:00:00Z')`, digest, digest, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES ('attempt-1', 'episode-1', 1, 'running', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func ledgerTestCall() Call {
	return Call{EpisodeID: "episode-1", CallID: "call-1", ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Arguments: EvidenceGetArguments{EntityID: "motor-1"}, Deadline: time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), AttemptID: "attempt-1", Fence: 1, Trace: traceForLedger(), MaxRows: 1, MaxBytes: 100, From: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), Until: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
}

func traceForLedger() contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
}
