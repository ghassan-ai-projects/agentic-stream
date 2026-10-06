package evidence

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"path/filepath"
	"testing"
	"time"
)

func mustCapabilities(t *testing.T, now time.Time, keys map[string][]byte) *Service {
	t.Helper()
	service, err := New(Config{Capabilities: &CapabilityConfig{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: func() time.Time { return now }}})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func mustCallService(t *testing.T, capabilities *Service, now time.Time, query Query) *Service {
	t.Helper()
	ledger, err := New(Config{Ledger: &LedgerConfig{DB: facadeDB(t), LeaseOwner: "owner", RuntimeEpoch: "epoch-1", Now: func() time.Time { return now }, OwnerCheck: func(context.Context, *sql.Tx, string) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{Calls: &CallConfig{Capabilities: capabilities, Ledger: ledger, RuntimeEpoch: "epoch-1", Now: func() time.Time { return now }, Query: query}})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func facadeDB(t *testing.T) *storage.DB {
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
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_attempt_id, current_fence, accepted_at) VALUES ('episode-1', 'scheduler-1', 'tenant-1', 'situation-1', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'running', 'attempt-1', 2, '2026-08-12T12:00:00Z')`, digest, digest, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES ('attempt-1', 'episode-1', 2, 'running', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return db
}
func facadeCall(token []byte) *runtimev1.EvidenceToolCall {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return &runtimev1.EvidenceToolCall{ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: "call-1", ToolName: "evidence.get", ArgumentsJson: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token, Deadline: timestamppb.New(now.Add(time.Minute)), AttemptId: "attempt-1", Fence: 2, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100, TimeFrom: timestamppb.New(now.Add(-time.Hour)), TimeUntil: timestamppb.New(now.Add(time.Hour))}
}
func TestFacadeRejectsMissingSafetyDependencies(t *testing.T) {
	db := facadeDB(t)
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	cases := map[string]Config{"empty": {}, "key": {Capabilities: &CapabilityConfig{Issuer: "runtime", Audience: "tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte("short")}}}, "owner": {Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", RuntimeEpoch: "epoch"}}, "epoch": {Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", OwnerCheck: owner}}, "calls": {Calls: &CallConfig{RuntimeEpoch: "epoch"}}}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg); err == nil {
				t.Fatal("missing dependency accepted")
			}
		})
	}
}
func TestFacadeKeysAreCopiedAndUnconfiguredOperationsRefuse(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	keys := map[string][]byte{"k1": key}
	service := mustCapabilities(t, now, keys)
	for i := range key {
		key[i] = 0
	}
	delete(keys, "k1")
	scope := Scope{EpisodeID: "e", AttemptID: "a", Fence: 1, TenantID: "t", SituationID: "s", SituationVersion: 1, EntityID: "x", Tools: []string{"evidence.get"}, From: now, Until: now, MaxRows: 1, MaxBytes: 100, Traceparent: "trace", RuntimeEpoch: "epoch"}
	token, err := service.Issue(scope)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Verify(token)
	if err != nil || got.KeyID != "k1" {
		t.Fatalf("scope=%+v err=%v", got, err)
	}
	if service.RuntimeEpoch() != "" || !service.IssueTime().Equal(now) {
		t.Fatal("capability-only identity/clock changed")
	}
	if _, err := service.Call(t.Context(), nil); err == nil {
		t.Fatal("nil call accepted")
	}
	if _, err := service.Call(t.Context(), facadeCall(token)); err == nil {
		t.Fatal("capability-only query accepted")
	}
	if err := service.ReclaimExpired(t.Context(), now); err == nil {
		t.Fatal("capability-only reclaim accepted")
	}
	if _, err := service.RecoverTx(t.Context(), nil, now); err == nil {
		t.Fatal("capability-only recovery accepted")
	}
	epoch, err := NewRuntimeEpoch()
	if err != nil || epoch == "" {
		t.Fatalf("epoch=%q err=%v", epoch, err)
	}
}
func TestFacadeRecoveryPreservesCallerTransactionAndOwnerFailure(t *testing.T) {
	db := facadeDB(t)
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
	now := time.Now().UTC()
	sentinel := errors.New("owner lost")
	var original *sql.Tx
	reject := false
	service, err := New(Config{Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", RuntimeEpoch: "new", OwnerCheck: func(_ context.Context, tx *sql.Tx, epoch string) error {
		if original != nil && tx != original {
			t.Fatal("different transaction")
		}
		if epoch != "new" {
			t.Fatal("different epoch")
		}
		if reject {
			return sentinel
		}
		return nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if service.RuntimeEpoch() != "new" {
		t.Fatal("epoch lost")
	}
	if _, err := service.Issue(Scope{}); err == nil {
		t.Fatal("unconfigured issuing accepted")
	}
	if _, err := service.Verify(nil); err == nil {
		t.Fatal("unconfigured verifying accepted")
	}
	rollback := errors.New("rollback")
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		original = tx
		if _, err := service.RecoverTx(t.Context(), tx, now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM evidence_call_ledger WHERE call_id = 'call-old'").Scan(&status); err != nil || status != "running" {
		t.Fatalf("recovery rollback=%s err=%v", status, err)
	}
	original = nil
	reject = true
	if err := service.ReclaimExpired(t.Context(), now); !errors.Is(err, sentinel) {
		t.Fatalf("owner failure=%v", err)
	}
	reject = false
	if err := service.ReclaimExpired(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := EventLogQuery(db)(t.Context(), Call{TenantID: "tenant-1", EntityID: "motor-1", From: now, Until: now, MaxRows: 1}); err != nil {
		t.Fatal(err)
	}
}
