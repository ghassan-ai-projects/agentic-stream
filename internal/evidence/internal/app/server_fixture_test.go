package app_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Scope = evidence.Scope
type Call = evidence.Call
type Query = evidence.Query
type QueryResult = evidence.QueryResult
type Server = evidence.Service

const maxArgumentsBytes = 256 << 10

func testCapabilities(t *testing.T, now time.Time) *evidence.Service {
	t.Helper()
	service, err := evidence.New(evidence.Config{Capabilities: &evidence.CapabilityConfig{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte("01234567890123456789012345678901")}, Now: func() time.Time { return now }}})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func effectiveEpoch(epoch string) string {
	if epoch == "" {
		return "epoch-1"
	}
	return epoch
}
func testServer(t *testing.T, db *storage.DB, capabilities *evidence.Service, now time.Time, fence int, epoch string, query Query) *Server {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET current_fence = ?", fence); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE episode_attempts SET fence = ?", fence); err != nil {
		t.Fatal(err)
	}
	ledger, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{DB: db, LeaseOwner: "owner-1", RuntimeEpoch: epoch, Now: func() time.Time { return now }, OwnerCheck: func(context.Context, *sql.Tx, string) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := evidence.New(evidence.Config{Calls: &evidence.CallConfig{Capabilities: capabilities, Ledger: ledger, Query: query, RuntimeEpoch: epoch, Now: func() time.Time { return now }}})
	if err != nil {
		t.Fatal(err)
	}
	return service
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
func readLedgerStatus(t *testing.T, db *storage.DB, callID string) (string, string) {
	t.Helper()
	var status string
	var code sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT status, error_code FROM evidence_call_ledger WHERE call_id = ?`, callID).Scan(&status, &code); err != nil {
		t.Fatalf("read ledger row %s: %v", callID, err)
	}
	return status, code.String
}
