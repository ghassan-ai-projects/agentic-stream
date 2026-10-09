package app_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var epoch0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func newService(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk sources.Clock, compiled *spec.CompiledSpec, tenantID string, cognition bool) (*app.Service, error) {
	return newServiceWithOwner(ctx, db, log, clk, compiled, tenantID, cognition, allowOwner)
}

func newServiceWithOwner(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk sources.Clock, compiled *spec.CompiledSpec, tenantID string, cognition bool, owner storage.OwnerCheck) (*app.Service, error) {
	return app.New(ctx, app.Config{Store: store.New(db, owner, "epoch", tenantID, compiled.Digest), Log: log, Clock: clk, Spec: compiled, TenantID: tenantID, Cognition: cognition})
}

type engineRig struct {
	db      *storage.DB
	log     *eventlog.EventLog
	service *app.Service
}

func newRig(t *testing.T, compiled spec.CompiledSpec) engineRig {
	t.Helper()
	return newRigWithCognition(t, compiled, false)
}

func newRigWithCognition(t *testing.T, compiled spec.CompiledSpec, cognition bool) engineRig {
	t.Helper()
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLog(db)
	service, err := newService(t.Context(), db, log, sources.Physical(), &compiled, "default", cognition)
	if err != nil {
		t.Fatal(err)
	}
	return engineRig{db: db, log: log, service: service}
}

func openDatabaseFile(t *testing.T) (string, *storage.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engine.db")
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return path, db
}

func countRows(t *testing.T, db *storage.DB, query string, args ...any) int {
	t.Helper()
	return queryValue[int](t, db, query, args...)
}

func queryText(t *testing.T, db *storage.DB, query string, args ...any) string {
	t.Helper()
	return queryValue[string](t, db, query, args...)
}

func queryValue[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func restartSpec() spec.CompiledSpec {
	return spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s"},
		Inputs:    []spec.Input{{Name: "level", EventType: "test.level", EntityType: "thing"}},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_max", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}, {Name: "watch", Severity: 30}, {Name: "warning", Severity: 60}}, Transitions: []spec.Transition{{From: "candidate", To: "watch", When: "features.level > 10", MinDuration: "0s"}, {From: "watch", To: "warning", When: "features.level > 10", MinDuration: "0s"}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
	}
}

func escalationSpec() spec.CompiledSpec {
	compiled := restartSpec()
	compiled.Situation.Transitions = []spec.Transition{
		{From: "candidate", To: "watch", When: "features.level > 10", MinDuration: "0s"},
		{From: "watch", To: "warning", When: "features.level > 50", MinDuration: "0s"},
	}
	return compiled
}

func heartbeatSpec() spec.CompiledSpec {
	return spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s"},
		Inputs:    []spec.Input{{Name: "heartbeat", EventType: "test.heartbeat", EntityType: "thing"}},
		Operators: []spec.Operator{{Name: "heartbeat_missing", Kind: "missing_heartbeat", Inputs: []string{"heartbeat"}, Duration: "5m", Output: "missing"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.missing == true"}, Reducers: []spec.Reducer{{Field: "facts.missing", Strategy: "latest_event_time", Input: "missing"}}},
	}
}

func thingEnvelope(id, eventType string, offset time.Duration, data map[string]any) contractsv1.Envelope {
	when := epoch0.Add(offset)
	return contractsv1.Envelope{ID: id, Type: eventType, SchemaVersion: "1.0", TenantID: "default", Source: "test", PartitionKey: "thing-1",
		Entity: contractsv1.EntityRef{Type: "thing", ID: "thing-1"}, EventTime: when, IngestedAt: when,
		Classification: contractsv1.ClassificationInternal, Data: data}
}

func appendEnvelope(t *testing.T, log *eventlog.EventLog, env contractsv1.Envelope) int {
	t.Helper()
	if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{env}); err != nil {
		t.Fatalf("append %s: %v", env.ID, err)
	}
	return env.PartitionID(0)
}

func appendLevel(t *testing.T, log *eventlog.EventLog, id string, offset time.Duration, level float64) int {
	t.Helper()
	return appendEnvelope(t, log, thingEnvelope(id, "test.level", offset, map[string]any{"level": level}))
}

func appendHeartbeat(t *testing.T, log *eventlog.EventLog, id string, offset time.Duration, bootID string) int {
	t.Helper()
	data := map[string]any{}
	if bootID != "" {
		data["boot_id"] = bootID
	}
	return appendEnvelope(t, log, thingEnvelope(id, "test.heartbeat", offset, data))
}

func runGlobal(t *testing.T, service *app.Service) int {
	t.Helper()
	processed, err := service.RunGlobal(t.Context(), nil)
	if err != nil {
		t.Fatalf("RunGlobal: %v", err)
	}
	return processed
}
