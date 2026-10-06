package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openService(t *testing.T) (*Service, store.Store, *storage.DB) {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	persistence := store.New(db)
	service := New(persistence)
	return service, persistence, db
}

func event(id string) contractsv1.CloudEvent {
	e := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/t", Type: "situation.version.published", Subject: "situation/s1", Time: now, DataContentType: "application/json", DataSchema: "urn:x", Data: map[string]any{"v": 1}, TenantID: "t", PartitionKey: "s1", IngestedTime: now, Classification: contractsv1.ClassificationInternal}
	e.EnvelopeDigest, _ = e.ComputeEnvelopeDigest()
	return e
}

func appendAll(t *testing.T, persistence store.Store, ids ...string) {
	t.Helper()
	for _, id := range ids {
		err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
			_, err := Append(t.Context(), tx, event(id), now)
			return err
		})
		if err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
}

func TestAppendRefusesRetiredIdentityAfterPrune(t *testing.T) {
	t.Parallel()
	service, persistence, _ := openService(t)
	appendAll(t, persistence, "old")
	later := now.Add(8 * 24 * time.Hour)
	if deleted, err := service.Prune(t.Context(), later, domain.RetentionFloor); err != nil || deleted != 1 {
		t.Fatalf("prune=%d err=%v", deleted, err)
	}
	fresh := event("old")
	fresh.Time, fresh.IngestedTime = later, later
	fresh.EnvelopeDigest, _ = fresh.ComputeEnvelopeDigest()
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		_, err := Append(t.Context(), tx, fresh, later)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with tombstone") {
		t.Fatalf("retired identity with a new payload error = %v", err)
	}
	same := event("old")
	err = persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		_, err := Append(t.Context(), tx, same, now.Add(time.Hour))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "already retired") {
		t.Fatalf("retired identity error = %v", err)
	}
}

func TestRacedInsertReleasesItsCursorOnlyForAnIdenticalWinner(t *testing.T) {
	t.Parallel()
	_, persistence, _ := openService(t)
	appendAll(t, persistence, "winner")
	sealed, err := domain.Seal(event("winner"), now)
	if err != nil {
		t.Fatal(err)
	}
	err = persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		cursor, err := tx.AllocateCursor(t.Context(), "t")
		if err != nil || cursor != 2 {
			t.Fatalf("cursor=%d err=%v", cursor, err)
		}
		if err := releaseRacedCursor(t.Context(), tx, sealed, cursor); err != nil {
			t.Fatalf("identical winner: %v", err)
		}
		again, _ := tx.AllocateCursor(t.Context(), "t")
		if again != 2 {
			t.Fatalf("released cursor was not reusable: %d", again)
		}
		sealed.SHA = []byte("different")
		if err := releaseRacedCursor(t.Context(), tx, sealed, again); err == nil {
			t.Fatal("conflicting winner accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPoisonSkipAuditsAndClearsTheCounterTogether(t *testing.T) {
	t.Parallel()
	service, persistence, db := openService(t)
	appendAll(t, persistence, "p")
	if _, err := db.ExecContext(t.Context(), "UPDATE notifications SET event_json = ?", []byte("{bad")); err != nil {
		t.Fatal(err)
	}
	request := domain.PageRequest{TenantID: "t", Limit: 5}
	for range domain.PoisonBudget - 1 {
		if _, err := service.ReadPage(t.Context(), request, now); !errors.Is(err, domain.ErrNotificationPoison) {
			t.Fatalf("poison pending error = %v", err)
		}
	}
	page, err := service.ReadPage(t.Context(), request, now)
	if err != nil || page.Skipped != 1 || page.NextCursor != 1 || len(page.Records) != 0 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	var attempts, audits int
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_poison_attempts").Scan(&attempts)
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits WHERE action = 'subscriber_skipped' AND requested_cursor = 1 AND oldest_cursor = 1").Scan(&audits)
	if attempts != 0 || audits != 1 {
		t.Fatalf("attempts=%d audits=%d", attempts, audits)
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
	service, persistence, db := openService(t)
	appendAll(t, persistence, "a", "b", "c")
	for _, test := range []struct {
		request domain.PageRequest
		want    error
		action  string
	}{
		{domain.PageRequest{TenantID: "t", Cursor: -1, Limit: 5}, domain.ErrCursorExpired, domain.AuditCursorExpired},
		{domain.PageRequest{TenantID: "t", Cursor: 0, Limit: 5, MaxLag: 1}, domain.ErrSubscriberTooSlow, domain.AuditSubscriberTooSlow},
	} {
		if _, err := service.ReadPage(t.Context(), test.request, now); !errors.Is(err, test.want) {
			t.Fatalf("%s: error = %v", test.action, err)
		}
		var requested, oldest int64
		query := "SELECT requested_cursor, oldest_cursor FROM notification_audits WHERE action = ?"
		if err := db.QueryRowContext(t.Context(), query, test.action).Scan(&requested, &oldest); err != nil || requested != test.request.Cursor || oldest != 1 {
			t.Fatalf("%s audit: requested=%d oldest=%d err=%v", test.action, requested, oldest, err)
		}
	}
}

func TestAppendLifecycleEventRefusesAnIncompleteRequest(t *testing.T) {
	t.Parallel()
	_, persistence, _ := openService(t)
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return AppendLifecycleEvent(context.Background(), tx, domain.LifecycleEvent{TenantID: "t"})
	})
	if err == nil || !strings.Contains(err.Error(), "identity is incomplete") {
		t.Fatalf("error = %v", err)
	}
}
