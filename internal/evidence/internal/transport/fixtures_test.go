package transport

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var fixedNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

const (
	traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	signingKey  = "01234567890123456789012345678901"
)

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

func validScope() domain.Scope {
	return domain.Scope{
		KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1, TenantID: "tenant-1",
		SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"},
		NotBefore: fixedNow, ExpiresAt: fixedNow.Add(10 * time.Minute),
		From: fixedNow.Add(-time.Hour), Until: fixedNow.Add(time.Hour), MaxRows: 10, MaxBytes: 1024,
		Traceparent: traceparent, RuntimeEpoch: "epoch-1",
	}
}

func validCall(token []byte, callID string) *runtimev1.EvidenceToolCall {
	return &runtimev1.EvidenceToolCall{
		ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: callID, ToolName: "evidence.get",
		ArgumentsJson: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token,
		Deadline: timestamppb.New(fixedNow.Add(time.Minute)), AttemptId: "attempt-1", Fence: 1, Traceparent: traceparent,
		TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100,
		TimeFrom: timestamppb.New(fixedNow.Add(-time.Hour)), TimeUntil: timestamppb.New(fixedNow.Add(time.Hour)),
	}
}

type workerFixture struct {
	server  *Server
	token   []byte
	queries *int
}

func newWorkerFixture(t *testing.T, query domain.Query) workerFixture {
	t.Helper()
	db := openLedgerDB(t)
	clock := func() time.Time { return fixedNow }
	capabilities, err := app.New(app.Config{Capabilities: &app.CapabilityConfig{
		Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte(signingKey)}, Now: clock,
	}})
	if err != nil {
		t.Fatalf("configure capabilities: %v", err)
	}
	token, err := capabilities.Issue(validScope())
	if err != nil {
		t.Fatalf("issue capability: %v", err)
	}
	ledger, err := app.New(app.Config{Ledger: &app.Ledger{
		Store:      store.New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1"),
		LeaseOwner: "owner", RuntimeEpoch: "epoch-1", Now: clock,
	}})
	if err != nil {
		t.Fatalf("configure ledger: %v", err)
	}
	queries := new(int)
	counted := func(ctx context.Context, call domain.Call) (domain.QueryResult, error) {
		*queries++
		return query(ctx, call)
	}
	service, err := app.New(app.Config{Calls: &app.CallConfig{Capabilities: capabilities, Ledger: ledger, Query: counted, RuntimeEpoch: "epoch-1", Now: clock}})
	if err != nil {
		t.Fatalf("configure calls: %v", err)
	}
	return workerFixture{server: New(service), token: token, queries: queries}
}

func okQuery(context.Context, domain.Call) (domain.QueryResult, error) {
	return domain.QueryResult{JSON: []byte(`{"ok":true}`)}, nil
}
