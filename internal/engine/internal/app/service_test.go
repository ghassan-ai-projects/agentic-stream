package app_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestNewRefusesMissingDependencies(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled := restartSpec()
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	log := eventlog.NewEventLog(db)
	cases := map[string]app.Config{
		"database": {Store: store.New(nil, owner, "e", "default", compiled.Digest), Log: log, Spec: &compiled, TenantID: "default"},
		"owner":    {Store: store.New(db, nil, "e", "default", compiled.Digest), Log: log, Spec: &compiled, TenantID: "default"},
		"log":      {Store: store.New(db, owner, "e", "default", compiled.Digest), Spec: &compiled, TenantID: "default"},
		"spec":     {Store: store.New(db, owner, "e", "default", ""), Log: log, TenantID: "default"},
	}
	for name, cfg := range cases {
		if service, err := app.New(ctx, cfg); err == nil || service != nil {
			t.Fatalf("engine without %s was constructed", name)
		}
	}
}

func TestRunStopsWithoutRuntimeOwnershipAndAppliesNothing(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled := restartSpec()
	log := eventlog.NewEventLog(db)
	lost := errors.New("ownership lost")
	owner := func(context.Context, *sql.Tx, string) error { return lost }
	service, err := app.New(ctx, app.Config{Store: store.New(db, owner, "epoch", "default", compiled.Digest), Log: log, Clock: clock.Physical(), Spec: &compiled, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, ctx, log, "evt-owned", 0, 15)
	if processed, err := service.RunGlobal(ctx, nil); !errors.Is(err, lost) || processed != 0 {
		t.Fatalf("processed=%d err=%v; an engine without ownership must apply nothing", processed, err)
	}
	var applied int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_inbox").Scan(&applied); err != nil || applied != 0 {
		t.Fatalf("inbox rows = %d err=%v", applied, err)
	}
}
