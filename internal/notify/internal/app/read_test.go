package app

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
)

func TestAMalformedRecordFailsThePageUntilItsBudgetIsSpentThenIsSkippedAndAudited(t *testing.T) {
	t.Parallel()
	corruptions := map[string][2]any{
		"digest does not match":          {[]byte(`{"id":"p"}`), make([]byte, 32)},
		"undecodable with a good digest": {[]byte("{bad"), canonicaljson.Sum([]byte("{bad"))},
		"decodable but not an event":     {[]byte("{}"), canonicaljson.Sum([]byte("{}"))},
	}
	for name, corruption := range corruptions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			service, persistence, db := openService(t)
			appendAll(t, persistence, "before", "poison", "after")
			if _, err := db.ExecContext(t.Context(), "UPDATE notifications SET event_json = ?, event_sha256 = ? WHERE event_id = 'poison'", corruption[0], corruption[1]); err != nil {
				t.Fatal(err)
			}
			request := domain.PageRequest{TenantID: "t", Limit: 5}
			for range domain.PoisonBudget - 1 {
				if _, err := service.ReadPage(t.Context(), request, now); !errors.Is(err, domain.ErrNotificationPoison) {
					t.Fatalf("poison pending error = %v", err)
				}
			}
			page, err := service.ReadPage(t.Context(), request, now)
			if err != nil || page.Skipped != 1 || page.NextCursor != 3 || len(page.Records) != 2 || page.Records[0].Cursor != 1 || page.Records[1].Cursor != 3 {
				t.Fatalf("page=%+v err=%v; want the poison cursor 2 skipped and its neighbors delivered", page, err)
			}
			var attempts, audits int
			_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_poison_attempts").Scan(&attempts)
			_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits WHERE action = 'subscriber_skipped' AND requested_cursor = 2 AND oldest_cursor = 2").Scan(&audits)
			if attempts != 0 || audits != 1 {
				t.Fatalf("attempts=%d audits=%d; want the counter cleared and one audit", attempts, audits)
			}
		})
	}
}

func TestValidRecordClearsEarlierPoisonAttempts(t *testing.T) {
	t.Parallel()
	service, persistence, db := openService(t)
	appendAll(t, persistence, "good")
	if _, err := persistence.Autocommit().CountPoisonAttempt(t.Context(), "t", 1, now); err != nil {
		t.Fatal(err)
	}
	if page, err := service.ReadPage(t.Context(), domain.PageRequest{TenantID: "t", Limit: 5}, now); err != nil || len(page.Records) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	var attempts int
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_poison_attempts").Scan(&attempts)
	if attempts != 0 {
		t.Fatalf("attempts after a valid read = %d", attempts)
	}
}

func TestRefusalsAreAuditedWithTheRequestedAndOldestCursors(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		request domain.PageRequest
		want    error
		action  string
	}{
		{"expired cursor", domain.PageRequest{TenantID: "t", Cursor: -1, Limit: 5}, domain.ErrCursorExpired, domain.AuditCursorExpired},
		{"slow subscriber", domain.PageRequest{TenantID: "t", Cursor: 0, Limit: 5, MaxLag: 1}, domain.ErrSubscriberTooSlow, domain.AuditSubscriberTooSlow},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, persistence, db := openService(t)
			appendAll(t, persistence, "a", "b", "c")
			if _, err := service.ReadPage(t.Context(), test.request, now); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			var requested, oldest int64
			query := "SELECT requested_cursor, oldest_cursor FROM notification_audits WHERE action = ?"
			if err := db.QueryRowContext(t.Context(), query, test.action).Scan(&requested, &oldest); err != nil || requested != test.request.Cursor || oldest != 1 {
				t.Fatalf("audit: requested=%d oldest=%d err=%v", requested, oldest, err)
			}
		})
	}
}

func TestAPageLimitOutsideItsBoundsIsRefusedWithoutAnAudit(t *testing.T) {
	t.Parallel()
	service, persistence, db := openService(t)
	appendAll(t, persistence, "a")
	for _, limit := range []int{-1, 0, domain.MaxPageLimit + 1} {
		if _, err := service.ReadPage(t.Context(), domain.PageRequest{TenantID: "t", Limit: limit}, now); err == nil {
			t.Errorf("limit %d accepted", limit)
		}
	}
	var audits int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits").Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("audits = %d, %v; want none for a malformed request", audits, err)
	}
}
