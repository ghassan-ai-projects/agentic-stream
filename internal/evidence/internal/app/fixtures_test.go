package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var testNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

const (
	testTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	testKey         = "01234567890123456789012345678901"
)

func fixedClock(at time.Time) func() time.Time { return func() time.Time { return at } }

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

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

func execSQL(t *testing.T, db *storage.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func ledgerOn(db *storage.DB, owner storage.OwnerCheck, epoch string) *Ledger {
	return &Ledger{Store: store.New(db, owner, epoch), LeaseOwner: "owner-1", RuntimeEpoch: epoch, Lease: time.Minute, Now: fixedClock(testNow)}
}

func newLedger(t *testing.T) (*Ledger, *storage.DB) {
	t.Helper()
	db := openLedgerDB(t)
	return ledgerOn(db, allowOwner, "epoch-1"), db
}

func ledgerTestCall() Call {
	return Call{
		EpisodeID: "episode-1", CallID: "call-1", ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1",
		SituationVersion: 1, EntityID: "motor-1", Arguments: EvidenceGetArguments{EntityID: "motor-1"},
		Deadline: testNow.Add(time.Minute), AttemptID: "attempt-1", Fence: 1,
		Trace:   contractsv1.TraceContext{Traceparent: testTraceparent},
		MaxRows: 1, MaxBytes: 100, From: testNow.Add(-time.Hour), Until: testNow,
	}
}

func reserveTestCall(t *testing.T, ledger *Ledger) ledgerReservation {
	t.Helper()
	reservation, err := ledger.Reserve(t.Context(), ledgerTestCall(), "token-1", ledger.RuntimeEpoch)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	return reservation
}

func ledgerStatus(t *testing.T, db *storage.DB, callID string) (status, code string) {
	t.Helper()
	var errorCode sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT status, error_code FROM evidence_call_ledger WHERE call_id = ?`, callID).Scan(&status, &errorCode); err != nil {
		t.Fatalf("read ledger row %s: %v", callID, err)
	}
	return status, errorCode.String
}

func ledgerRows(t *testing.T, db *storage.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM evidence_call_ledger").Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}

func capabilityService(t *testing.T, now time.Time) *Service {
	t.Helper()
	return capabilityServiceWith(t, CapabilityConfig{Now: fixedClock(now)})
}

func capabilityServiceWith(t *testing.T, overrides CapabilityConfig) *Service {
	t.Helper()
	cfg := CapabilityConfig{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte(testKey)}}
	if overrides.Issuer != "" {
		cfg.Issuer = overrides.Issuer
	}
	if overrides.Audience != "" {
		cfg.Audience = overrides.Audience
	}
	if overrides.KeyID != "" {
		cfg.KeyID = overrides.KeyID
	}
	if overrides.Keys != nil {
		cfg.Keys = overrides.Keys
	}
	cfg.Now, cfg.MaxTTL, cfg.ClockSkew = overrides.Now, overrides.MaxTTL, overrides.ClockSkew
	service, err := New(Config{Capabilities: &cfg})
	if err != nil {
		t.Fatalf("configure capabilities: %v", err)
	}
	return service
}

func grantedScope() Scope {
	return Scope{
		KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1, TenantID: "tenant-1",
		SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"},
		NotBefore: testNow, ExpiresAt: testNow.Add(10 * time.Minute),
		From: testNow.Add(-time.Hour), Until: testNow.Add(time.Hour), MaxRows: 10, MaxBytes: 1024,
		Traceparent: testTraceparent, RuntimeEpoch: "epoch-1",
	}
}

func workerEnvelope(token []byte, callID string) domain.Envelope {
	return domain.Envelope{
		ProtocolVersion: "1.0", EpisodeID: "episode-1", CallID: callID, ToolName: "evidence.get", TenantID: "tenant-1",
		SituationID: "situation-1", EntityID: "motor-1", AttemptID: "attempt-1", Fence: 1, SituationVersion: 1,
		MaxRows: 1, MaxBytes: 100, ArgumentsJSON: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token,
		Deadline:    domain.Timestamp{Value: testNow.Add(time.Minute), Present: true, Valid: true},
		From:        domain.Timestamp{Value: testNow.Add(-time.Hour), Present: true, Valid: true},
		Until:       domain.Timestamp{Value: testNow.Add(time.Hour), Present: true, Valid: true},
		Traceparent: testTraceparent,
	}
}

type callOptions struct {
	runtimeEpoch string
	serviceNow   time.Time
	owner        storage.OwnerCheck
	query        Query
}

type callFixture struct {
	service *Service
	db      *storage.DB
	token   []byte
	queries *int
}

func newCallFixture(t *testing.T, options callOptions) *callFixture {
	t.Helper()
	if options.runtimeEpoch == "" {
		options.runtimeEpoch = "epoch-1"
	}
	if options.serviceNow.IsZero() {
		options.serviceNow = testNow
	}
	if options.owner == nil {
		options.owner = allowOwner
	}
	if options.query == nil {
		options.query = func(context.Context, Call) (QueryResult, error) {
			return QueryResult{JSON: []byte(`{"rows":[{"value":42}]}`), RowCount: 1}, nil
		}
	}
	db := openLedgerDB(t)
	issuer := capabilityService(t, testNow)
	token, err := issuer.Issue(grantedScope())
	if err != nil {
		t.Fatalf("issue capability: %v", err)
	}
	queries := new(int)
	counted := func(ctx context.Context, call Call) (QueryResult, error) {
		*queries++
		return options.query(ctx, call)
	}
	ledger, err := New(Config{Ledger: &Ledger{Store: store.New(db, options.owner, options.runtimeEpoch), LeaseOwner: "owner-1", RuntimeEpoch: options.runtimeEpoch, Now: fixedClock(options.serviceNow)}})
	if err != nil {
		t.Fatalf("configure ledger: %v", err)
	}
	verifier := capabilityService(t, options.serviceNow)
	service, err := New(Config{Calls: &CallConfig{Capabilities: verifier, Ledger: ledger, Query: counted, RuntimeEpoch: options.runtimeEpoch, Now: fixedClock(options.serviceNow)}})
	if err != nil {
		t.Fatalf("configure calls: %v", err)
	}
	return &callFixture{service: service, db: db, token: token, queries: queries}
}

func requireRefusal(t *testing.T, err error, want domain.ErrorKind) {
	t.Helper()
	_ = refusalFrom(t, err, want)
}

func refusalFrom(t *testing.T, err error, want domain.ErrorKind) *domain.Refusal {
	t.Helper()
	var refusal *domain.Refusal
	if !errors.As(err, &refusal) || refusal.Kind != want {
		t.Fatalf("error = %v, want a %s refusal", err, want)
	}
	return refusal
}

func requireContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func tamperedToken(token []byte) []byte {
	tampered := append([]byte(nil), token...)
	middle := len(tampered) / 2
	tampered[middle] = 'A'
	if token[middle] == 'A' {
		tampered[middle] = 'B'
	}
	return tampered
}
