package evidence

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var facadeNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

const (
	facadeKey         = "01234567890123456789012345678901"
	facadeTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func facadeClock() func() time.Time { return func() time.Time { return facadeNow } }

func newCapabilities(t *testing.T) *Service {
	t.Helper()
	service, err := New(Config{Capabilities: &CapabilityConfig{
		Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte(facadeKey)}, Now: facadeClock(),
	}})
	if err != nil {
		t.Fatalf("configure capabilities: %v", err)
	}
	return service
}

func newLedger(t *testing.T, db *storage.DB, owner storage.OwnerCheck, epoch string) *Service {
	t.Helper()
	service, err := New(Config{Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", RuntimeEpoch: epoch, Now: facadeClock(), OwnerCheck: owner}})
	if err != nil {
		t.Fatalf("configure ledger: %v", err)
	}
	return service
}

func newCallService(t *testing.T, capabilities *Service, query Query) *Service {
	t.Helper()
	service, err := New(Config{Calls: &CallConfig{
		Capabilities: capabilities, Ledger: newLedger(t, facadeDB(t), allowOwner, "epoch-1"), RuntimeEpoch: "epoch-1", Now: facadeClock(), Query: query,
	}})
	if err != nil {
		t.Fatalf("configure calls: %v", err)
	}
	return service
}

func facadeDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	digest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_attempt_id, current_fence, accepted_at) VALUES ('episode-1', 'scheduler-1', 'tenant-1', 'situation-1', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'running', 'attempt-1', 2, '2026-08-12T12:00:00Z')`, digest, digest, []byte(`{}`)); err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES ('attempt-1', 'episode-1', 2, 'running', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	return db
}

func facadeScope() Scope {
	return Scope{
		KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 2, TenantID: "tenant-1", SituationID: "situation-1",
		SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"}, NotBefore: facadeNow, ExpiresAt: facadeNow.Add(10 * time.Minute),
		From: facadeNow.Add(-time.Hour), Until: facadeNow.Add(time.Hour), MaxRows: 10, MaxBytes: 1024, Traceparent: facadeTraceparent, RuntimeEpoch: "epoch-1",
	}
}

func facadeCall(token []byte) *runtimev1.EvidenceToolCall {
	return &runtimev1.EvidenceToolCall{
		ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: "call-1", ToolName: "evidence.get", ArgumentsJson: []byte(`{"entity_id":"motor-1"}`),
		CapabilityToken: token, Deadline: timestamppb.New(facadeNow.Add(time.Minute)), AttemptId: "attempt-1", Fence: 2, Traceparent: facadeTraceparent,
		TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100,
		TimeFrom: timestamppb.New(facadeNow.Add(-time.Hour)), TimeUntil: timestamppb.New(facadeNow.Add(time.Hour)),
	}
}

func TestNewRefusesConfigurationMissingASafetyDependency(t *testing.T) {
	t.Parallel()
	db := facadeDB(t)
	tests := map[string]Config{
		"empty":                   {},
		"short signing key":       {Capabilities: &CapabilityConfig{Issuer: "runtime", Audience: "tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte("short")}}},
		"ledger without an owner": {Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", RuntimeEpoch: "epoch"}},
		"ledger without an epoch": {Ledger: &LedgerConfig{DB: db, LeaseOwner: "owner", OwnerCheck: allowOwner}},
		"calls without ports":     {Calls: &CallConfig{RuntimeEpoch: "epoch"}},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if service, err := New(cfg); err == nil || service != nil {
				t.Fatalf("New() = %v, %v, want a refusal", service, err)
			}
		})
	}
}

func TestIssuedCapabilityVerifiesThroughTheFacade(t *testing.T) {
	t.Parallel()
	service := newCapabilities(t)
	token, err := service.Issue(facadeScope())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := service.Verify(token)
	if err != nil || got.KeyID != "k1" || got.EntityID != "motor-1" || got.Fence != 2 {
		t.Fatalf("scope = %+v, err %v", got, err)
	}
	if !service.IssueTime().Equal(facadeNow) || service.RuntimeEpoch() != "" {
		t.Fatalf("clock %v epoch %q, want the configured clock and no epoch", service.IssueTime(), service.RuntimeEpoch())
	}
}

func TestOperationsOutsideTheConfiguredResponsibilityRefuse(t *testing.T) {
	t.Parallel()
	service := newCapabilities(t)
	token, err := service.Issue(facadeScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(t.Context(), nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("nil call error = %v, want FailedPrecondition", err)
	}
	if _, err := service.Call(t.Context(), facadeCall(token)); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("capability-only call error = %v, want FailedPrecondition", err)
	}
	if err := service.ReclaimExpired(t.Context(), facadeNow); err == nil {
		t.Fatal("a capability-only service reclaimed calls")
	}
	if _, err := service.RecoverTx(t.Context(), nil, facadeNow); err == nil {
		t.Fatal("a capability-only service recovered calls")
	}
	ledgerOnly := newLedger(t, facadeDB(t), allowOwner, "epoch-1")
	if _, err := ledgerOnly.Issue(Scope{}); err == nil {
		t.Fatal("a ledger-only service issued a capability")
	}
	if _, err := ledgerOnly.Verify(nil); err == nil {
		t.Fatal("a ledger-only service verified a capability")
	}
}

func TestWorkerCallIsServedAndRefusedWithGRPCStatuses(t *testing.T) {
	t.Parallel()
	capabilities := newCapabilities(t)
	token, err := capabilities.Issue(facadeScope())
	if err != nil {
		t.Fatal(err)
	}
	service := newCallService(t, capabilities, func(context.Context, Call) (QueryResult, error) {
		return QueryResult{JSON: []byte(`{"rows":[]}`)}, nil
	})
	result, err := service.Call(t.Context(), facadeCall(token))
	if err != nil || string(result.GetResultJson()) != `{"rows":[]}` {
		t.Fatalf("result = %v, err %v", result, err)
	}
	foreign := facadeCall(token)
	foreign.CallId, foreign.TenantId = "call-2", "tenant-2"
	if _, err := service.Call(t.Context(), foreign); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("out-of-scope call error = %v, want PermissionDenied", err)
	}
}

func TestRecoveryJoinsTheCallersTransactionAndKeepsOwnerFailures(t *testing.T) {
	t.Parallel()
	db := facadeDB(t)
	insertRunningCall(t, db, "epoch-old")
	sentinel := errors.New("owner lost")
	var original *sql.Tx
	rejectOwner := false
	service := newLedger(t, db, func(_ context.Context, tx *sql.Tx, epoch string) error {
		if original != nil && tx != original {
			t.Error("ownership was asserted on a different transaction")
		}
		if epoch != "epoch-new" {
			t.Errorf("ownership asserted for epoch %q", epoch)
		}
		if rejectOwner {
			return sentinel
		}
		return nil
	}, "epoch-new")

	rollback := errors.New("rollback")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		original = tx
		if recovered, err := service.RecoverTx(t.Context(), tx, facadeNow); err != nil || recovered != 1 {
			t.Errorf("recovered = %d, err %v, want 1", recovered, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v", err)
	}
	if status := callStatus(t, db, "call-old"); status != "running" {
		t.Fatalf("status = %q, want running after the caller rolled back", status)
	}

	original, rejectOwner = nil, true
	if err := service.ReclaimExpired(t.Context(), facadeNow); !errors.Is(err, sentinel) {
		t.Fatalf("reclaim error = %v, want the owner failure", err)
	}
	rejectOwner = false
	if err := service.ReclaimExpired(t.Context(), facadeNow); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if status := callStatus(t, db, "call-old"); status != "interrupted" {
		t.Fatalf("status = %q, want interrupted", status)
	}
}

func TestEventLogQueryAnswersAnEmptyWindowWithAnEmptyRowSet(t *testing.T) {
	t.Parallel()
	result, err := EventLogQuery(facadeDB(t))(t.Context(), Call{TenantID: "tenant-1", EntityID: "motor-1", From: facadeNow, Until: facadeNow, MaxRows: 1})
	if err != nil || string(result.JSON) != `{"rows":[]}` || result.RowCount != 0 {
		t.Fatalf("result = %s (rows %d), err %v", result.JSON, result.RowCount, err)
	}
}

func TestNewRuntimeEpochIsOpaqueAndUnique(t *testing.T) {
	t.Parallel()
	first, err := NewRuntimeEpoch()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntimeEpoch()
	if err != nil || first == "" || first == second {
		t.Fatalf("epochs %q and %q, err %v, want distinct non-empty values", first, second, err)
	}
}

func insertRunningCall(t *testing.T, db *storage.DB, epoch string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO evidence_call_ledger (
			tenant_id, episode_id, attempt_id, fence, call_id, token_id, runtime_epoch,
			tool_name, situation_id, situation_version, entity_id, time_from, time_until,
			max_rows, max_bytes, deadline, request_sha256, status, lease_owner,
			lease_until, reserved_at
		) VALUES ('tenant-1', 'episode-1', 'attempt-1', 2, 'call-old', 'token-old', ?,
			'evidence.get', 'situation-1', 1, 'motor-1', '2026-08-12T11:00:00.000000000Z',
			'2026-08-12T12:00:00.000000000Z', 1, 100, '2026-08-12T12:01:00.000000000Z', zeroblob(32), 'running',
			'owner', '2026-08-12T12:00:00.000000000Z', '2026-08-12T11:59:00.000000000Z')`, epoch); err != nil {
		t.Fatalf("insert running call: %v", err)
	}
}

func callStatus(t *testing.T, db *storage.DB, callID string) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM evidence_call_ledger WHERE call_id = ?", callID).Scan(&status); err != nil {
		t.Fatalf("read call %s: %v", callID, err)
	}
	return status
}
