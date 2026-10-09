package store_test

import (
	"fmt"
	"testing"
)

func TestRowsAreReadAfterACursorInOrderUpToTheLimitWithinOneTenant(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	insertRows(t, tx, "a", "b", "c")
	rows, err := tx.ReadRows(t.Context(), "t", 1, 5)
	if err != nil || len(rows) != 2 || rows[0].Cursor != 2 || rows[1].Cursor != 3 {
		t.Fatalf("rows=%+v err=%v; want cursors 2 and 3", rows, err)
	}
	if rows, err := tx.ReadRows(t.Context(), "t", 0, 1); err != nil || len(rows) != 1 || rows[0].Cursor != 1 {
		t.Fatalf("limited rows=%+v err=%v; want only cursor 1", rows, err)
	}
	if rows, err := tx.ReadRows(t.Context(), "other", 0, 5); err != nil || len(rows) != 0 {
		t.Fatalf("another tenant's rows=%+v err=%v; want none", rows, err)
	}
}

func TestTenantBoundsReportTheOldestRetainedCursorAndTheHighwater(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if bounds, err := tx.TenantBounds(t.Context(), "t"); err != nil || bounds.HasOldest || bounds.HasNextCursor {
		t.Fatalf("empty bounds = %+v, %v; want none", bounds, err)
	}
	if _, err := tx.InsertNotification(t.Context(), row("a", 4, at)); err != nil {
		t.Fatal(err)
	}
	if bounds, err := tx.TenantBounds(t.Context(), "t"); err != nil || !bounds.HasOldest || bounds.Oldest != 4 || bounds.HasNextCursor {
		t.Fatalf("bounds=%+v err=%v; want the oldest cursor 4 and no highwater", bounds, err)
	}
}

func TestPoisonAttemptsCountPerCursorUntilCleared(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	for _, cursor := range []int64{4, 5} {
		if inserted, err := tx.InsertNotification(t.Context(), row(fmt.Sprintf("poison-%d", cursor), cursor, at)); err != nil || !inserted {
			t.Fatalf("insert cursor %d: inserted=%v err=%v", cursor, inserted, err)
		}
	}
	for want := 1; want <= 3; want++ {
		if got, err := tx.CountPoisonAttempt(t.Context(), "t", 4, at); err != nil || got != want {
			t.Fatalf("attempt %d = %d err=%v", want, got, err)
		}
	}
	if got, err := tx.CountPoisonAttempt(t.Context(), "t", 5, at); err != nil || got != 1 {
		t.Fatalf("another cursor's first attempt = %d, %v; want 1", got, err)
	}
	if err := tx.ClearPoisonAttempts(t.Context(), "t", 4); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.CountPoisonAttempt(t.Context(), "t", 4, at); err != nil || got != 1 {
		t.Fatalf("attempts after clear = %d, %v; want 1", got, err)
	}
}
