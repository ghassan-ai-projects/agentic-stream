package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTheLogHeadAndTheGlobalPageReadSearchTheTenantPositionIndex(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	storagetest.RequireIndexedPlan(t, db, "event_log_tenant_position", currentPositionSQL, "tenant")
	query, args := recordQuery(domain.ReadRequest{TenantID: "tenant", PartitionID: -1, AfterPosition: 10, Limit: 100})
	storagetest.RequireIndexedPlan(t, db, "event_log_tenant_position", query, args...)
}
