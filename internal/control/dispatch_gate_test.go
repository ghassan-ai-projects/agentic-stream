package control_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestDispatchCapabilityReadsCurrentInterlockWithoutUpstreamCallback(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	allow := func(context.Context, *sql.Tx) error { return nil }
	if _, err := interlock.Clear(ctx, db, allow, "operator", time.Now()); err != nil {
		t.Fatal(err)
	}
	gate := runtimecontrol.NewDispatchAuthorization(db, interlock.DurableReader{}, "tenant", "target")
	if err := gate.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := interlock.Trip(ctx, db, "operator", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Check(ctx); !errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("cached readiness allowed a later trip: %v", err)
	}
	var version int64
	if err := db.QueryRowContext(ctx, "SELECT version FROM runtime_interlock WHERE singleton_id=1").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("read-only gate mutated interlock: version=%d", version)
	}
	for _, misconfigured := range []struct {
		database bool
		reader   bool
	}{{false, true}, {true, false}} {
		dbArg := db
		if !misconfigured.database {
			dbArg = nil
		}
		var reader interlock.Reader = interlock.DurableReader{}
		if !misconfigured.reader {
			reader = nil
		}
		if err := runtimecontrol.NewDispatchAuthorization(dbArg, reader, "tenant", "target").Check(ctx); err == nil {
			t.Fatal("misconfigured gate allowed dispatch")
		}
	}
}
