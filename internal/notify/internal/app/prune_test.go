package app

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
)

func TestPruneAndPrunableRefuseRetentionBelowTheFloor(t *testing.T) {
	t.Parallel()
	service, _, _ := openService(t)
	short := domain.RetentionFloor - time.Nanosecond
	if _, err := service.Prune(t.Context(), now, short); err == nil {
		t.Error("Prune accepted a retention below the floor")
	}
	if _, err := service.Prunable(t.Context(), now, short); err == nil {
		t.Error("Prunable accepted a retention below the floor")
	}
}

func TestPrunableCountsOnlyNotificationsOlderThanTheRetentionWithoutRetiringThem(t *testing.T) {
	t.Parallel()
	service, persistence, _ := openService(t)
	appendAll(t, persistence, "old")
	if _, err := appendEvent(t, persistence, event("recent"), now.Add(6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	later := now.Add(8 * 24 * time.Hour)
	for range 2 {
		if count, err := service.Prunable(t.Context(), later, domain.RetentionFloor); err != nil || count != 1 {
			t.Fatalf("prunable = %d, %v; want only the old notification, however often it is counted", count, err)
		}
	}
	if deleted, err := service.Prune(t.Context(), later, domain.RetentionFloor); err != nil || deleted != 1 {
		t.Fatalf("pruned = %d, %v; want the one old notification", deleted, err)
	}
}
