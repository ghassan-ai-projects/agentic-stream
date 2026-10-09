package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNewRefusesMissingDependencies(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := restartSpec()
	log := eventlog.NewEventLog(db)
	cases := map[string]app.Config{
		"database": {Store: store.New(nil, allowOwner, "e", "default", compiled.Digest), Log: log, Spec: &compiled, TenantID: "default"},
		"owner":    {Store: store.New(db, nil, "e", "default", compiled.Digest), Log: log, Spec: &compiled, TenantID: "default"},
		"log":      {Store: store.New(db, allowOwner, "e", "default", compiled.Digest), Spec: &compiled, TenantID: "default"},
		"spec":     {Store: store.New(db, allowOwner, "e", "default", ""), Log: log, TenantID: "default"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			service, err := app.New(t.Context(), cfg)
			if err == nil || service != nil || !strings.Contains(err.Error(), "engine requires a database, runtime owner check, event log and compiled spec") {
				t.Fatalf("engine without %s: service=%v err=%v, want the constructor refusal", name, service, err)
			}
		})
	}
}

func TestNewNamesAnInvalidSpecInsteadOfBuildingPlanes(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	invalid := restartSpec()
	invalid.Windows = []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "soon", Emit: "early_and_close"}}
	_, err := newService(t.Context(), db, eventlog.NewEventLog(db), sources.Physical(), &invalid, "default", false)
	if err == nil || !strings.Contains(err.Error(), "operator runtime: window tiny") {
		t.Fatalf("err = %v, want operator runtime: window tiny", err)
	}
}

func TestNewRequiresSchemaValidationOnTheLogWhenEveryInputDeclaresASchema(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLog(db)
	compiled := restartSpec()
	compiled.Inputs[0].SchemaRef = "test.level/1.0"
	if _, err := newService(t.Context(), db, log, sources.Physical(), &compiled, "default", false); err != nil {
		t.Fatal(err)
	}
	event := thingEnvelope("evt-1", "test.level", 0, map[string]any{"level": 15.0})
	if _, err := log.Append(t.Context(), "default", []contractsv1.Envelope{event}); err == nil || !strings.Contains(err.Error(), "is not registered") {
		t.Fatalf("err = %v, want the log to demand a registered schema", err)
	}
}

func TestRunStopsWithoutRuntimeOwnershipAndAppliesNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := restartSpec()
	log := eventlog.NewEventLog(db)
	lost := errors.New("ownership lost")
	owner := func(context.Context, *sql.Tx, string) error { return lost }
	service, err := newServiceWithOwner(t.Context(), db, log, sources.Physical(), &compiled, "default", false, owner)
	if err != nil {
		t.Fatal(err)
	}
	appendLevel(t, log, "evt-owned", 0, 15)
	if processed, err := service.RunGlobal(t.Context(), nil); !errors.Is(err, lost) || processed != 0 {
		t.Fatalf("processed=%d err=%v; an engine without ownership must apply nothing", processed, err)
	}
	for _, table := range []string{"event_inbox", "situations", "partition_checkpoints"} {
		if got := countRows(t, db, "SELECT COUNT(*) FROM "+table); got != 0 {
			t.Errorf("%s holds %d rows, want none", table, got)
		}
	}
}
