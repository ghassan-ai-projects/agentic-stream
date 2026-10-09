package store

import (
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func seedRecoveryState(t *testing.T) (*storage.DB, time.Time) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	digest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status,
			current_attempt_id, current_fence, accepted_at
		) VALUES ('episode-old', 'scheduler-old', 'tenant-1', 'situation-1', 1,
			'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'running',
			'attempt-old', 1, '2026-08-12T12:00:00Z')`, digest, digest); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at)
		VALUES ('attempt-old', 'episode-old', 1, 'running', 'epoch-old', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO evidence_call_ledger (
			tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch,
			tool_name, situation_id, situation_version, entity_id, time_from, time_until,
			max_rows, max_bytes, deadline, request_sha256, status, lease_owner,
			lease_until, reserved_at
		) VALUES ('tenant-1', 'episode-old', 'attempt-old', 1, 'call-old', 'token-old', 'epoch-old',
			'evidence.get', 'situation-1', 1, 'motor-1', '2026-08-12T11:00:00Z',
			'2026-08-12T12:00:00Z', 1, 100, '2026-08-12T12:01:00Z', ?, 'running',
			'instance-old', '2026-08-12T12:10:00Z', '2026-08-12T12:00:00Z')`, digest); err != nil {
		t.Fatalf("insert evidence call: %v", err)
	}
	return db, time.Date(2026, 8, 12, 12, 2, 0, 0, time.UTC)
}

func newRecoveryCoordinator(t *testing.T, db *storage.DB, now time.Time, overrideClock bool) *RecoveryCoordinator {
	t.Helper()
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-new", Lease: time.Minute, Now: func() time.Time { return now }}
	ledger, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{OwnerCheck: owner.Assert, DB: db, LeaseOwner: "instance-new", RuntimeEpoch: "epoch-new", Lease: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &RecoveryCoordinator{Owner: owner, Ledger: ledger, Epoch: "epoch-new"}
	if overrideClock {
		coordinator.Now = func() time.Time { return now }
	}
	return coordinator
}

func recoveryStatuses(t *testing.T, db *storage.DB) (attempt, evidenceCall string) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM episode_attempts WHERE attempt_id = 'attempt-old'").Scan(&attempt); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM evidence_call_ledger WHERE call_id = 'call-old'").Scan(&evidenceCall); err != nil {
		t.Fatalf("read evidence call: %v", err)
	}
	return attempt, evidenceCall
}
