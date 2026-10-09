package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestExpiringWatchesSearchOnlyActiveWatches(t *testing.T) {
	t.Parallel()
	now := "2026-01-01T00:00:00.000000000Z"
	storagetest.RequireIndexedPlan(t, storagetest.OpenTemp(t), "watch_conditions_active_expiry", expireDueSQL, now, now)
}
