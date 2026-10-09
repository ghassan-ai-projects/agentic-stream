package notify_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func TestPruningPreservesHighwaterAndRejectsExpiredEvents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	original := testEvent("original", baseTime)
	if _, err := appendEvent(ctx, db, original, baseTime); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if deleted, err := outbox.Prune(ctx, later, 7*24*time.Hour); err != nil || deleted != 1 {
		t.Fatalf("prune = %d, %v", deleted, err)
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, later); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("old cursor = %v", err)
	}
	if _, err := appendEvent(ctx, db, original, later); !errors.Is(err, notify.ErrEventExpired) {
		t.Fatalf("expired event = %v", err)
	}
	if cursor, err := appendEvent(ctx, db, testEvent("next", later), later); err != nil || cursor != 2 {
		t.Fatalf("next cursor = %d, %v", cursor, err)
	}
}

func TestPruneRefusesRetentionBelowTheFloor(t *testing.T) {
	t.Parallel()
	_, outbox := openOutbox(t)
	if _, err := outbox.Prune(t.Context(), baseTime, 24*time.Hour); err == nil {
		t.Fatal("one-day retention accepted")
	}
}

// The three retention steps commit together: if the last one fails, nothing is
// retired and the notification is still readable.
func TestPruneIsOneTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("kept", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_tombstone_expiry BEFORE DELETE ON notification_event_tombstones BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_event_tombstones (tenant_id, event_id, event_sha256, retired_at) VALUES ('tenant', 'old', ?, '2000-01-01T00:00:00Z')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if _, err := outbox.Prune(ctx, later, 7*24*time.Hour); err == nil {
		t.Fatal("prune succeeded despite the injected failure")
	}
	var notifications, tombstones int
	if err := db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM notifications), (SELECT COUNT(*) FROM notification_event_tombstones)").Scan(&notifications, &tombstones); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 || tombstones != 1 {
		t.Fatalf("after a failed prune: notifications=%d tombstones=%d, want 1 and 1 (nothing retired)", notifications, tombstones)
	}
}

func TestPrunableCountsWithoutRetiring(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("old", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if count, err := outbox.Prunable(ctx, later, 7*24*time.Hour); err != nil || count != 1 {
		t.Fatalf("prunable = %d, %v", count, err)
	}
	if count, err := outbox.Prunable(ctx, later, 7*24*time.Hour); err != nil || count != 1 {
		t.Fatalf("a count changed the outbox: %d, %v", count, err)
	}
}
