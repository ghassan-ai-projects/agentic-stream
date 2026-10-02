package control_test

import (
	"database/sql"
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestDispatchCapabilityReadsCurrentInterlockWithoutUpstreamCallback(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	set := func(status string, version int64) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return interlock.Set(ctx, tx, status, "operator", version, "now") })
	}
	if err := set("ready", 2); err != nil {
		t.Fatal(err)
	}
	gate := runtimecontrol.NewDispatchAuthorization(db, interlock.DurableReader{}, "tenant", "target")
	if err := gate.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := set("tripped", 3); err != nil {
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
