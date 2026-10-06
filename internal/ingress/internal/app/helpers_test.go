package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type connector struct {
	run func(context.Context) (int, error)
}

func (c connector) Run(ctx context.Context) (int, error) { return c.run(ctx) }

func newApp(t *testing.T, db *storage.DB, log *eventlog.EventLog, tenant string) *app.Service {
	t.Helper()
	service, err := app.New(app.Config{Store: store.New(db), Log: log, TenantID: tenant})
	if err != nil {
		t.Fatalf("new ingress service: %v", err)
	}
	return service
}

func newJSONL(t *testing.T, db *storage.DB, log *eventlog.EventLog, tenant, path, connectorID string) connector {
	service := newApp(t, db, log, tenant)
	return connector{run: func(ctx context.Context) (int, error) { return service.ReplayJSONL(ctx, path, connectorID) }}
}

func newSimulator(t *testing.T, db *storage.DB, options domain.SimulatorOptions, path, connectorID string) connector {
	service := newApp(t, db, eventlog.NewEventLog(db), options.TenantID)
	return connector{run: func(ctx context.Context) (int, error) {
		return service.ReplaySimulator(ctx, options, path, connectorID)
	}}
}
