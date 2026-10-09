package store_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const stamp = "2026-10-08T10:00:00.000000000Z"

func inTx(t *testing.T, db *storage.DB, fn func(*sql.Tx) error) error {
	t.Helper()
	return db.WithTx(t.Context(), fn)
}

func readState(t *testing.T, db *storage.DB) (state domain.State, err error) {
	t.Helper()
	err = inTx(t, db, func(tx *sql.Tx) (err error) {
		state, err = store.Read(t.Context(), tx)
		return err
	})
	return state, err
}

func TestReadReturnsTheSeededReadyInterlock(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	state, err := readState(t, db)
	if err != nil || state.Status != domain.StatusReady || state.Version != 1 {
		t.Fatalf("Read = %+v, %v; want the ready interlock at version 1", state, err)
	}
}

func TestChangeMovesTheInterlockOneVersionAtATime(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	var initial, tripped, cleared, read domain.State
	err := inTx(t, db, func(tx *sql.Tx) (err error) {
		if initial, err = store.Read(t.Context(), tx); err != nil {
			return err
		}
		if tripped, err = store.Change(t.Context(), tx, domain.StatusTripped, "operator stop", stamp); err != nil {
			return err
		}
		if cleared, err = store.Change(t.Context(), tx, domain.StatusReady, "inspected", stamp); err != nil {
			return err
		}
		read, err = store.Read(t.Context(), tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if tripped.Version != initial.Version+1 || cleared.Version != initial.Version+2 {
		t.Fatalf("versions = %d, %d after %d, want one step each", tripped.Version, cleared.Version, initial.Version)
	}
	if read != cleared || read.Status != domain.StatusReady || read.Reason != "inspected" || read.UpdatedAt != stamp {
		t.Fatalf("Read after Change = %+v, want the cleared state %+v", read, cleared)
	}
}

func TestChangeRefusesAnInvalidChangeAndWritesNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	err := inTx(t, db, func(tx *sql.Tx) error {
		_, err := store.Change(t.Context(), tx, domain.StatusTripped, "", stamp)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "change interlock to tripped") || !strings.Contains(err.Error(), "required") {
		t.Fatalf("Change without a reason = %v, want the domain message under the change context", err)
	}
	if state, err := readState(t, db); err != nil || state.Status != domain.StatusReady || state.Version != 1 {
		t.Fatalf("a refused change moved the interlock: %+v, %v", state, err)
	}
}

func TestAssertAdmitsReadyAndRefusesTrippedWithItsReason(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if err := inTx(t, db, func(tx *sql.Tx) error { return store.Assert(t.Context(), tx) }); err != nil {
		t.Fatalf("Assert on the seeded interlock = %v, want nil", err)
	}
	if err := inTx(t, db, func(tx *sql.Tx) error { return store.Set(t.Context(), tx, domain.StatusTripped, "test stop", 2, stamp) }); err != nil {
		t.Fatalf("trip: %v", err)
	}
	err := inTx(t, db, func(tx *sql.Tx) error { return store.Assert(t.Context(), tx) })
	if !errors.Is(err, domain.ErrTripped) || !strings.Contains(err.Error(), "test stop") {
		t.Fatalf("Assert on a tripped interlock = %v, want ErrTripped carrying the reason", err)
	}
}

func TestSetRefusesAVersionThatIsNotNewer(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	set := func(status, reason string, version int64) error {
		return inTx(t, db, func(tx *sql.Tx) error { return store.Set(t.Context(), tx, status, reason, version, stamp) })
	}
	if err := set(domain.StatusTripped, "first", 2); err != nil {
		t.Fatalf("trip to version 2: %v", err)
	}
	for name, version := range map[string]int64{"the version already stored": 2, "an older version": 1} {
		err := set(domain.StatusReady, "stale reopen", version)
		if err == nil || !strings.Contains(err.Error(), "row was not updated") {
			t.Errorf("Set to %s = %v, want the not updated error", name, err)
		}
	}
	if state, err := readState(t, db); err != nil || state.Status != domain.StatusTripped || state.Reason != "first" {
		t.Fatalf("a stale Set changed the interlock: %+v, %v", state, err)
	}
}

func TestSetRefusesInvalidValuesWithTheDomainMessage(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	err := inTx(t, db, func(tx *sql.Tx) error {
		return store.Set(t.Context(), tx, "paused", "r", 2, stamp)
	})
	if err == nil || !strings.Contains(err.Error(), `invalid interlock status "paused"`) {
		t.Fatalf("Set with an unknown status = %v, want the domain message", err)
	}
}

func TestAssertFailsWhenTheInterlockRowIsMissing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := db.ExecContext(t.Context(), "DELETE FROM runtime_interlock"); err != nil {
		t.Fatalf("remove the interlock row: %v", err)
	}
	err := inTx(t, db, func(tx *sql.Tx) error { return store.Assert(t.Context(), tx) })
	if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "read runtime interlock") {
		t.Fatalf("Assert without the row = %v, want the read error wrapping sql.ErrNoRows", err)
	}
}
