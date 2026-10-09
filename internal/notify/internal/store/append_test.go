package store_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

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
		if again, _ := tx.AllocateCursor(t.Context(), "t"); again != 2 {
			t.Fatalf("reallocated cursor = %d", again)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCursorsAreIndependentPerTenant(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	for _, tenant := range []string{"t", "t", "other"} {
		if _, err := tx.AllocateCursor(t.Context(), tenant); err != nil {
			t.Fatal(err)
		}
	}
	if bounds, err := tx.TenantBounds(t.Context(), "other"); err != nil || bounds.NextCursor != 2 {
		t.Fatalf("other tenant next cursor = %+v, %v; want 2 after one allocation", bounds, err)
	}
	if bounds, err := tx.TenantBounds(t.Context(), "t"); err != nil || bounds.NextCursor != 3 {
		t.Fatalf("tenant next cursor = %+v, %v; want 3 after two allocations", bounds, err)
	}
}

func TestAnIdentityIsInsertedOnceAndFoundByItsCursorAndDigest(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	insertRows(t, tx, "a", "b", "c")
	if inserted, err := tx.InsertNotification(t.Context(), row("a", 9, at)); err != nil || inserted {
		t.Fatalf("duplicate identity inserted=%v err=%v", inserted, err)
	}
	stored, found, err := tx.FindNotification(t.Context(), "t", "b")
	if err != nil || !found || stored.Cursor != 2 || stored.SHA[0] != 2 {
		t.Fatalf("stored=%+v found=%v err=%v", stored, found, err)
	}
	if _, found, err := tx.FindNotification(t.Context(), "t", "missing"); err != nil || found {
		t.Fatalf("missing identity found=%v err=%v", found, err)
	}
	if _, found, err := tx.FindNotification(t.Context(), "other", "b"); err != nil || found {
		t.Fatalf("another tenant's identity found=%v err=%v", found, err)
	}
}

func TestTheWinnerOfARacedInsertIsReadByDigestAndCursor(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	insertRows(t, tx, "winner")
	if digest, err := tx.StoredSHA(t.Context(), "t", "winner"); err != nil || digest[0] != 1 {
		t.Fatalf("StoredSHA = %v, %v; want the winner's digest", digest, err)
	}
	if cursor, err := tx.NotificationCursor(t.Context(), "t", "winner"); err != nil || cursor != 1 {
		t.Fatalf("NotificationCursor = %d, %v; want 1", cursor, err)
	}
	if _, err := tx.StoredSHA(t.Context(), "t", "missing"); err == nil {
		t.Fatal("the digest of a missing notification was read")
	}
	if _, err := tx.NotificationCursor(t.Context(), "t", "missing"); err == nil {
		t.Fatal("the cursor of a missing notification was read")
	}
}
