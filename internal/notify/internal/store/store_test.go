package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var at = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) (store.Store, *storage.DB) {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db), db
}

func sha(seed int64) []byte {
	digest := make([]byte, 32)
	digest[0] = byte(seed & 0xff)
	return digest
}

func row(eventID string, cursor int64, created time.Time) store.Notification {
	return store.Notification{TenantID: "t", EventID: eventID, EventType: "x", Cursor: cursor, EventJSON: []byte("{}"), EventSHA: sha(cursor), CreatedAt: created}
}

func TestStoreReportsConfiguration(t *testing.T) {
	t.Parallel()
	if store.New(nil).Configured() {
		t.Fatal("nil database reported as configured")
	}
	persistence, _ := openStore(t)
	if !persistence.Configured() {
		t.Fatal("database not reported as configured")
	}
}

func TestJoinedTransactionKeepsCallerOwnership(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	rollback := errors.New("caller rolls back")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		unit := store.Join(tx)
		cursor, err := unit.AllocateCursor(t.Context(), "t")
		if err != nil || cursor != 1 {
			t.Fatalf("cursor=%d err=%v", cursor, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("err=%v", err)
	}
	if bounds, err := persistence.Autocommit().TenantBounds(t.Context(), "t"); err != nil || bounds.HasNextCursor {
		t.Fatalf("rolled-back cursor persisted: %+v err=%v", bounds, err)
	}
}

func TestCursorsAreGaplessAcrossReleasedAllocations(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		first, _ := tx.AllocateCursor(t.Context(), "t")
		second, _ := tx.AllocateCursor(t.Context(), "t")
		if first != 1 || second != 2 {
			t.Fatalf("cursors = %d, %d", first, second)
		}
		if err := tx.ReleaseCursor(t.Context(), "t", second); err != nil {
			t.Fatal(err)
		}
		if err := tx.ReleaseCursor(t.Context(), "t", second); err == nil {
			t.Fatal("releasing an already released cursor succeeded")
		}
		again, _ := tx.AllocateCursor(t.Context(), "t")
		if again != 2 {
			t.Fatalf("reallocated cursor = %d", again)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInsertFindAndReadRowsInCursorOrder(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	for i, id := range []string{"a", "b", "c"} {
		inserted, err := tx.InsertNotification(t.Context(), row(id, int64(i+1), at))
		if err != nil || !inserted {
			t.Fatalf("insert %s: inserted=%v err=%v", id, inserted, err)
		}
	}
	if inserted, err := tx.InsertNotification(t.Context(), row("a", 9, at)); err != nil || inserted {
		t.Fatalf("duplicate identity inserted=%v err=%v", inserted, err)
	}
	stored, found, err := tx.FindNotification(t.Context(), "t", "b")
	if err != nil || !found || stored.Cursor != 2 {
		t.Fatalf("stored=%+v found=%v err=%v", stored, found, err)
	}
	if _, found, _ := tx.FindNotification(t.Context(), "t", "missing"); found {
		t.Fatal("missing identity found")
	}
	rows, err := tx.ReadRows(t.Context(), "t", 1, 5)
	if err != nil || len(rows) != 2 || rows[0].Cursor != 2 || rows[1].Cursor != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows, _ := tx.ReadRows(t.Context(), "t", 0, 1); len(rows) != 1 {
		t.Fatalf("limit not applied: %d rows", len(rows))
	}
}

func TestTenantBoundsAndPoisonCounting(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if _, err := tx.InsertNotification(t.Context(), row("a", 4, at)); err != nil {
		t.Fatal(err)
	}
	bounds, err := tx.TenantBounds(t.Context(), "t")
	if err != nil || !bounds.HasOldest || bounds.Oldest != 4 || bounds.HasNextCursor {
		t.Fatalf("bounds=%+v err=%v", bounds, err)
	}
	for want := 1; want <= 3; want++ {
		if got, err := tx.CountPoisonAttempt(t.Context(), "t", 4, at); err != nil || got != want {
			t.Fatalf("attempt %d = %d err=%v", want, got, err)
		}
	}
	if err := tx.ClearPoisonAttempts(t.Context(), "t", 4); err != nil {
		t.Fatal(err)
	}
	if got, _ := tx.CountPoisonAttempt(t.Context(), "t", 4, at); got != 1 {
		t.Fatalf("attempts after clear = %d", got)
	}
}

func TestRetentionRetiresDeletesAndExpiresTombstones(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	tx := persistence.Autocommit()
	if _, err := tx.InsertNotification(t.Context(), row("old", 1, at)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.InsertNotification(t.Context(), row("new", 2, at.Add(48*time.Hour))); err != nil {
		t.Fatal(err)
	}
	cutoff, now := at.Add(time.Hour), at.Add(72*time.Hour)
	if err := tx.RetireNotifications(t.Context(), now, cutoff); err != nil {
		t.Fatal(err)
	}
	if digest, found, err := tx.FindTombstone(t.Context(), "t", "old"); err != nil || !found || len(digest) != 32 {
		t.Fatalf("tombstone sha=%v found=%v err=%v", digest, found, err)
	}
	if _, found, _ := tx.FindTombstone(t.Context(), "t", "new"); found {
		t.Fatal("recent notification tombstoned")
	}
	if deleted, err := tx.DeleteRetiredNotifications(t.Context(), cutoff); err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	if err := tx.DeleteExpiredTombstones(t.Context(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var tombstones int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_event_tombstones").Scan(&tombstones); err != nil || tombstones != 0 {
		t.Fatalf("tombstones=%d err=%v", tombstones, err)
	}
}

func TestAuditRowsRecordRefusals(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	audit := store.Audit{ID: "a1", TenantID: "t", Action: "cursor_expired", Requested: 3, Oldest: 5, Details: []byte(`{}`), At: at}
	if err := persistence.Autocommit().RecordAudit(t.Context(), audit); err != nil {
		t.Fatal(err)
	}
	var action string
	var requested, oldest int64
	if err := db.QueryRowContext(t.Context(), "SELECT action, requested_cursor, oldest_cursor FROM notification_audits WHERE audit_id = 'a1'").Scan(&action, &requested, &oldest); err != nil {
		t.Fatal(err)
	}
	if action != "cursor_expired" || requested != 3 || oldest != 5 {
		t.Fatalf("audit = %s %d %d", action, requested, oldest)
	}
}
