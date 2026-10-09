package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

func TestAppendingTheSameEventAgainReturnsItsCursorAndADifferentPayloadIsRefused(t *testing.T) {
	t.Parallel()
	_, persistence, _ := openService(t)
	for range 2 {
		if cursor, err := appendEvent(t, persistence, event("same"), now); err != nil || cursor != 1 {
			t.Fatalf("append cursor = %d, %v; want the existing cursor 1", cursor, err)
		}
	}
	changed := event("same")
	changed.Data = map[string]any{"v": 2}
	changed.EnvelopeDigest, _ = changed.ComputeEnvelopeDigest()
	if _, err := appendEvent(t, persistence, changed, now); err == nil || !strings.Contains(err.Error(), "same") {
		t.Fatalf("err = %v, want the payload conflict to name the event", err)
	}
	if cursor, err := appendEvent(t, persistence, event("next"), now); err != nil || cursor != 2 {
		t.Fatalf("next cursor = %d, %v; want 2: neither the duplicate nor the conflict consumed a cursor", cursor, err)
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
	if _, err := appendEvent(t, persistence, fresh, later); err == nil || !strings.Contains(err.Error(), "conflicts with tombstone") {
		t.Fatalf("retired identity with a new payload error = %v", err)
	}
	if _, err := appendEvent(t, persistence, event("old"), now.Add(time.Hour)); err == nil || !strings.Contains(err.Error(), "already retired") {
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

func TestAnIncompleteLifecycleRequestIsRefusedBeforeAnythingIsStored(t *testing.T) {
	t.Parallel()
	_, persistence, db := openService(t)
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return AppendLifecycleEvent(context.Background(), tx, domain.LifecycleEvent{TenantID: "t"})
	})
	if err == nil || !strings.Contains(err.Error(), "identity is incomplete") {
		t.Fatalf("error = %v", err)
	}
	var stored int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notifications").Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("stored notifications = %d, %v; want none", stored, err)
	}
}

func TestPublishingAnEventThatLostTheRaceReturnsTheWinnersCursorAndKeepsCursorsGapless(t *testing.T) {
	t.Parallel()
	_, persistence, _ := openService(t)
	appendAll(t, persistence, "winner")
	sealed, err := domain.Seal(event("winner"), now)
	if err != nil {
		t.Fatal(err)
	}
	err = persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		cursor, err := publish(t.Context(), tx, sealed, now)
		if err != nil || cursor != 1 {
			t.Fatalf("raced publish = cursor %d err=%v, want the winner's cursor 1", cursor, err)
		}
		if next, _ := tx.AllocateCursor(t.Context(), "t"); next != 2 {
			t.Fatalf("next cursor = %d, want 2: the raced allocation must be given back", next)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
