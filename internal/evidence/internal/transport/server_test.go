package transport

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var fixedNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func openLedgerDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storagetest.Open(t.Context(), filepath.Join(dir, "runtime.db"))
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
func validCall(token []byte, callID string) *runtimev1.EvidenceToolCall {
	return &runtimev1.EvidenceToolCall{ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: callID, ToolName: "evidence.get", ArgumentsJson: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token, Deadline: timestamppb.New(fixedNow.Add(time.Minute)), AttemptId: "attempt-1", Fence: 1, Traceparent: traceparent, TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100, TimeFrom: timestamppb.New(fixedNow.Add(-time.Hour)), TimeUntil: timestamppb.New(fixedNow.Add(time.Hour))}
}
func validScope() domain.Scope {
	return domain.Scope{KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"}, NotBefore: fixedNow, ExpiresAt: fixedNow.Add(10 * time.Minute), From: fixedNow.Add(-time.Hour), Until: fixedNow.Add(time.Hour), MaxRows: 10, MaxBytes: 1024, Traceparent: traceparent, RuntimeEpoch: "epoch-1"}
}
func TestGRPCConversionAndDurableReplay(t *testing.T) {
	db := openLedgerDB(t)
	clock := func() time.Time { return fixedNow }
	caps, err := app.New(app.Config{Capabilities: &app.CapabilityConfig{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte("01234567890123456789012345678901")}, Now: clock}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := caps.Issue(validScope())
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := app.New(app.Config{Ledger: &app.Ledger{Store: store.New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1"), LeaseOwner: "owner", RuntimeEpoch: "epoch-1", Now: clock}})
	if err != nil {
		t.Fatal(err)
	}
	queries := 0
	service, err := app.New(app.Config{Calls: &app.CallConfig{Capabilities: caps, Ledger: ledger, Query: func(context.Context, domain.Call) (domain.QueryResult, error) {
		queries++
		return domain.QueryResult{JSON: []byte(`{"ok":true}`)}, nil
	}, RuntimeEpoch: "epoch-1", Now: clock}})
	if err != nil {
		t.Fatal(err)
	}
	server := New(service)
	for range 2 {
		result, err := server.Call(t.Context(), validCall(token, "call"))
		if err != nil || string(result.GetResultJson()) != `{"ok":true}` || len(result.GetResultSha256()) != 32 {
			t.Fatalf("result=%v err=%v", result, err)
		}
	}
	if queries != 1 {
		t.Fatal("replayed query executed")
	}
	if _, err := server.Call(t.Context(), nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	req := validCall(token, "other")
	req.TenantId = "other"
	if _, err := server.Call(t.Context(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	for kind, want := range map[domain.ErrorKind]codes.Code{domain.InvalidArgument: codes.InvalidArgument, domain.ResourceExhausted: codes.ResourceExhausted, domain.AlreadyExists: codes.AlreadyExists, domain.Internal: codes.Internal, domain.Canceled: codes.Canceled, domain.DeadlineExceeded: codes.DeadlineExceeded} {
		if got := status.Code(statusError(domain.Refuse(kind, "safe"))); got != want {
			t.Fatalf("kind=%s got=%s", kind, got)
		}
	}
	if got := status.Convert(statusError(errors.New("private database details"))); got.Code() != codes.Internal || got.Message() != "evidence query failed" {
		t.Fatal(got)
	}
}
func TestEventLogProviderPreservesExactScopedBytes(t *testing.T) {
	db := openLedgerDB(t)
	now := fixedNow
	events := []contractsv1.Envelope{{ID: "evt-1", Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "default", Source: "test", PartitionKey: "motor-1", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-1"}, EventTime: now, IngestedAt: now, Classification: contractsv1.ClassificationInternal, Data: map[string]any{"celsius": 42.0}}}
	if _, err := eventlog.NewEventLog(db).Append(t.Context(), "default", events); err != nil {
		t.Fatal(err)
	}
	query := EventLogQuery(db)
	call := domain.Call{TenantID: "default", EntityID: "motor-1", From: now, Until: now.Add(time.Second), MaxRows: 1}
	result, err := query(t.Context(), call)
	const want = `{"rows":[{"data":{"celsius":42},"event_id":"evt-1","event_time":"2026-08-12T12:00:00Z","event_type":"sensor.temperature"}]}`
	if err != nil || result.RowCount != 1 || string(result.JSON) != want {
		t.Fatalf("result=%s err=%v", result.JSON, err)
	}
	call.TenantID = "other"
	result, err = query(t.Context(), call)
	if err != nil || string(result.JSON) != `{"rows":[]}` {
		t.Fatalf("foreign=%s err=%v", result.JSON, err)
	}
	if _, err := eventRow(eventlog.EntityEvent{Payload: []byte("bad")}); err == nil {
		t.Fatal("invalid payload accepted")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := query(t.Context(), call); err == nil {
		t.Fatal("closed database queried")
	}
}

func TestUnknownRefusalFailsClosed(t *testing.T) {
	if got := status.Code(statusError(domain.Refuse(domain.ErrorKind("unknown"), "safe"))); got != codes.Internal {
		t.Fatalf("unknown refusal=%s", got)
	}
}
