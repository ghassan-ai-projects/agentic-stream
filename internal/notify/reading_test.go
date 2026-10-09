package notify_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func TestNewRequiresDatabase(t *testing.T) {
	t.Parallel()
	if outbox, err := notify.New(nil); err == nil || outbox != nil {
		t.Fatalf("New(nil) = %v, %v", outbox, err)
	}
}

func TestNotificationsAreCursorResumableAndAuditExpiredCursor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	for i, id := range []string{"evt-1", "evt-2"} {
		if _, err := appendEvent(ctx, db, testEvent(id, baseTime.Add(time.Duration(i)*time.Second)), baseTime); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 2 || page.Records[0].Cursor != 1 || page.Records[1].Cursor != 2 || page.NextCursor != 2 {
		t.Fatalf("page=%v err=%v", page, err)
	}
	page, err = outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Cursor: 1, Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 1 || page.Records[0].Event.ID != "evt-2" {
		t.Fatalf("resumed page=%v err=%v", page, err)
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Cursor: -1, Limit: 10}, baseTime); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
	if audits := countAudits(t, db, "cursor_expired"); audits != 1 {
		t.Fatalf("expired audits=%d", audits)
	}
}

func TestSlowSubscriberIsAuditedAndDisconnected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	for _, id := range []string{"a", "b", "c"} {
		if _, err := appendEvent(ctx, db, testEvent(id, baseTime), baseTime); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10, MaxLag: 2}, baseTime); !errors.Is(err, notify.ErrSubscriberTooSlow) {
		t.Fatalf("slow subscriber error = %v", err)
	}
	if audits := countAudits(t, db, "subscriber_too_slow"); audits != 1 {
		t.Fatalf("slow audits=%d", audits)
	}
}

func TestReadPageRefusesLimitsOutsideBounds(t *testing.T) {
	t.Parallel()
	_, outbox := openOutbox(t)
	for _, limit := range []int{0, 1001} {
		if _, err := outbox.ReadPage(t.Context(), notify.PageRequest{TenantID: "tenant", Limit: limit}, baseTime); err == nil {
			t.Errorf("limit %d accepted", limit)
		}
	}
}

func TestPoisonNotificationRetriesBeforeAuditedSkip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("poison", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notifications SET event_json = ? WHERE tenant_id = ? AND cursor = 1", []byte("{bad"), "tenant"); err != nil {
		t.Fatal(err)
	}
	request := notify.PageRequest{TenantID: "tenant", Limit: 10}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := outbox.ReadPage(ctx, request, baseTime); !errors.Is(err, notify.ErrNotificationPoison) {
			t.Fatalf("attempt %d error=%v", attempt, err)
		}
	}
	page, err := outbox.ReadPage(ctx, request, baseTime)
	if err != nil || page.Skipped != 1 || page.NextCursor != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if audits := countAudits(t, db, "subscriber_skipped"); audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}
