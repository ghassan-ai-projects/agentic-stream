package interlock_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestTripBlocksAndClearReopensTheActionPlane(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "interlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	reader := interlock.DurableReader{}
	assert := func() error {
		return db.WithTx(t.Context(), func(tx *sql.Tx) error { return reader.Assert(t.Context(), tx, "tenant", "motor/1", "R1") })
	}
	change := func(apply func(context.Context, *sql.Tx, string, string) (interlock.State, error), reason string) interlock.State {
		var state interlock.State
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			var err error
			state, err = apply(t.Context(), tx, reason, now)
			return err
		}); err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		return state
	}

	if err := assert(); err != nil {
		t.Fatalf("the seeded interlock must be ready: %v", err)
	}
	tripped := change(interlock.Trip, "operator stop")
	if tripped.Status != "tripped" || tripped.Version != 2 || tripped.Reason != "operator stop" {
		t.Fatalf("trip = %+v", tripped)
	}
	if err := assert(); !errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("a tripped interlock must refuse: %v", err)
	}
	cleared := change(interlock.Clear, "inspected")
	if cleared.Status != "ready" || cleared.Version != 3 {
		t.Fatalf("clear = %+v", cleared)
	}
	if err := assert(); err != nil {
		t.Fatalf("a cleared interlock must admit: %v", err)
	}
}

func TestChangesNeedAReason(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "interlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.Trip(t.Context(), tx, "", "2026-08-12T12:00:00Z")
		return err
	})
	if err == nil {
		t.Fatal("a trip without a reason was accepted")
	}
}
