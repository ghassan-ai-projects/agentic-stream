package app_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// newService builds the engine with an ownership check that always passes, as
// replay does, for tests that exercise stream processing rather than ownership.
func newService(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk sources.Clock, compiled *spec.CompiledSpec, tenantID string, cognition bool) (*app.Service, error) {
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	return app.New(ctx, app.Config{Store: store.New(db, owner, "epoch", tenantID, compiled.Digest), Log: log, Clock: clk, Spec: compiled, TenantID: tenantID, Cognition: cognition})
}

func appendObservedEvent(t *testing.T, log *eventlog.EventLog, eventType, source string, entity contractsv1.EntityRef, data map[string]any) {
	t.Helper()
	env := contractsv1.Envelope{
		ID:             "evt-1",
		Type:           eventType,
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         source,
		PartitionKey:   entity.ID,
		Entity:         entity,
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal,
		Data:           data,
	}
	if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{env}); err != nil {
		t.Fatalf("append event: %v", err)
	}
}
