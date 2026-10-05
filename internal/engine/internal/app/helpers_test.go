package app_test

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// newService builds the engine with an ownership check that always passes, as
// replay does, for tests that exercise stream processing rather than ownership.
func newService(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string, cognition bool) (*app.Service, error) {
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	return app.New(ctx, app.Config{Store: store.New(db, owner, "epoch", tenantID, compiled.Digest), Log: log, Clock: clk, Spec: compiled, TenantID: tenantID, Cognition: cognition})
}
