package interlock_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func allow(context.Context, *sql.Tx) error { return nil }

func admits(t *testing.T, db *storage.DB) error {
	t.Helper()
	return db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return interlock.Assert(t.Context(), tx)
	})
}

func TestTripBlocksAndClearReopensTheActionPlane(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if err := admits(t, db); err != nil {
		t.Fatalf("the seeded interlock must be ready: %v", err)
	}
	tripped, err := interlock.Trip(t.Context(), db, "operator stop", now)
	want := interlock.State{Status: "tripped", Reason: "operator stop", UpdatedAt: kernel.FormatTime(now), Version: 2}
	if err != nil || tripped != want {
		t.Fatalf("Trip = %+v, %v; want %+v", tripped, err, want)
	}
	if err := admits(t, db); !errors.Is(err, interlock.ErrTripped) || !strings.Contains(err.Error(), "operator stop") {
		t.Fatalf("a tripped interlock must refuse with its reason: %v", err)
	}
	cleared, err := interlock.Clear(t.Context(), db, allow, "inspected", now)
	if err != nil || cleared.Status != "ready" || cleared.Version != 3 || cleared.Reason != "inspected" {
		t.Fatalf("Clear = %+v, %v", cleared, err)
	}
	if err := admits(t, db); err != nil {
		t.Fatalf("a cleared interlock must admit: %v", err)
	}
	if status, err := interlock.Status(t.Context(), db); err != nil || status != cleared {
		t.Fatalf("Status = %+v, %v; want %+v", status, err, cleared)
	}
}

func TestTripInAndClearInChangeTheInterlockInsideTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	var tripped, cleared interlock.State
	err := db.WithTx(t.Context(), func(tx *sql.Tx) (err error) {
		if tripped, err = interlock.TripIn(t.Context(), tx, "in tx", now); err != nil {
			return err
		}
		if err = interlock.Assert(t.Context(), tx); !errors.Is(err, interlock.ErrTripped) {
			t.Errorf("the caller's transaction must see its own trip: %v", err)
		}
		cleared, err = interlock.ClearIn(t.Context(), tx, "in tx cleared", now)
		return err
	})
	if err != nil || tripped.Status != "tripped" || cleared.Status != "ready" || cleared.Version != tripped.Version+1 {
		t.Fatalf("TripIn = %+v, ClearIn = %+v, err = %v", tripped, cleared, err)
	}
}

func TestClearIsRefusedWithoutTheFenceAndWritesNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := interlock.Trip(t.Context(), db, "operator stop", now); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("not the owner")
	refuse := func(context.Context, *sql.Tx) error { return denied }
	if _, err := interlock.Clear(t.Context(), db, refuse, "too early", now); !errors.Is(err, denied) {
		t.Fatalf("a clear the fence refused = %v, want the fence's error", err)
	}
	if _, err := interlock.Clear(t.Context(), db, nil, "no fence", now); err == nil || !strings.Contains(err.Error(), "a fence is required") {
		t.Fatalf("a clear without a fence = %v, want the fence is required error", err)
	}
	status, err := interlock.Status(t.Context(), db)
	if err != nil || status.Status != "tripped" || status.Version != 2 {
		t.Fatalf("a refused clear changed the interlock: %+v, %v", status, err)
	}
}

func TestChangesNeedAReasonAndWriteNothingWithout(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := interlock.Trip(t.Context(), db, "", now); err == nil || !strings.Contains(err.Error(), "reason, version, and timestamp are required") {
		t.Fatalf("a trip without a reason = %v, want the required-fields error", err)
	}
	if _, err := interlock.Clear(t.Context(), db, allow, "", now); err == nil || !strings.Contains(err.Error(), "reason, version, and timestamp are required") {
		t.Fatalf("a clear without a reason = %v, want the required-fields error", err)
	}
	if status, err := interlock.Status(t.Context(), db); err != nil || status.Status != "ready" || status.Version != 1 {
		t.Fatalf("a refused change moved the interlock: %+v, %v", status, err)
	}
}
