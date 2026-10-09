package control_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestDispatchGateRefusesOnceTheInterlockTripsAndNeverWritesTheInterlock(t *testing.T) {
	t.Parallel()
	db := openOwnerDB(t)
	allow := func(context.Context, *sql.Tx) error { return nil }
	if _, err := interlock.Clear(t.Context(), db, allow, "operator", epoch0); err != nil {
		t.Fatalf("clear interlock: %v", err)
	}
	gate := runtimecontrol.NewDispatchAuthorization(db)
	if err := gate.Check(t.Context()); err != nil {
		t.Fatalf("a cleared interlock must let dispatch through: %v", err)
	}
	if _, err := interlock.Trip(t.Context(), db, "operator", epoch0); err != nil {
		t.Fatalf("trip interlock: %v", err)
	}
	if err := gate.Check(t.Context()); !errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("the gate must read the current interlock, not a cached one: %v", err)
	}
	var version int64
	if err := db.QueryRowContext(t.Context(), "SELECT version FROM runtime_interlock WHERE singleton_id=1").Scan(&version); err != nil {
		t.Fatalf("read interlock version: %v", err)
	}
	if version != 3 {
		t.Fatalf("the read-only gate mutated the interlock: version=%d, want 3 (initial, clear, trip)", version)
	}
}

func TestDispatchGateWithoutADatabaseRefusesDispatch(t *testing.T) {
	t.Parallel()
	assertRefusal(t, runtimecontrol.NewDispatchAuthorization(nil).Check(t.Context()), "dispatch readiness gate is not configured")
}
