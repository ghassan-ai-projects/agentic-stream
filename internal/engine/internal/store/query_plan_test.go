package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTimerLookupsSearchOnlyPendingTimers(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	storagetest.RequireIndexedPlan(t, db, "timers_pending_due", dueTimerPartitionsSQL, "deployment", "tenant", "2026-01-01T00:00:00.000000000Z")
	storagetest.RequireIndexedPlan(t, db, "timers_pending_state", cancelPendingHeartbeatSQL, "deployment", "tenant", 0, "hb", "m1")
}
