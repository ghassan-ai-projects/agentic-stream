package store_test

import (
	"testing"
	"time"
)

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
	if count, err := tx.CountNotificationsBefore(t.Context(), cutoff); err != nil || count != 1 {
		t.Fatalf("retirable = %d, %v; want only the old notification", count, err)
	}
	if err := tx.RetireNotifications(t.Context(), now, cutoff); err != nil {
		t.Fatal(err)
	}
	if digest, found, err := tx.FindTombstone(t.Context(), "t", "old"); err != nil || !found || len(digest) != 32 {
		t.Fatalf("tombstone sha=%v found=%v err=%v", digest, found, err)
	}
	if _, found, err := tx.FindTombstone(t.Context(), "t", "new"); err != nil || found {
		t.Fatalf("recent notification tombstoned: found=%v err=%v", found, err)
	}
	if deleted, err := tx.DeleteRetiredNotifications(t.Context(), cutoff); err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	if err := tx.DeleteExpiredTombstones(t.Context(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM notification_event_tombstones"); n != 0 {
		t.Fatalf("tombstones=%d, want the expired tombstone deleted", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM notifications"); n != 1 {
		t.Fatalf("notifications=%d, want only the recent one kept", n)
	}
}

func TestRetiringTwiceKeepsTheFirstTombstone(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	tx := persistence.Autocommit()
	if _, err := tx.InsertNotification(t.Context(), row("old", 1, at)); err != nil {
		t.Fatal(err)
	}
	for _, now := range []time.Time{at.Add(2 * time.Hour), at.Add(3 * time.Hour)} {
		if err := tx.RetireNotifications(t.Context(), now, at.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM notification_event_tombstones"); n != 1 {
		t.Fatalf("tombstones=%d, want one", n)
	}
}
