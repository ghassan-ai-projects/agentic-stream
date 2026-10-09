package storagetest_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRequireIndexedPlanAcceptsAnIndexedSearch(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	storagetest.RequireIndexedPlan(t, db, "event_log_tenant_position", "SELECT event_id FROM event_log WHERE tenant_id = ? AND position > ? ORDER BY position", "default", 0)
}
