package app

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
)

func TestARestartedSchedulerQueuesEveryAdmittedTriggerUnderItsOwnItem(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	h.process(candidateVersion("sit-1", 1, 15))
	restarted, err := New(Config{DeploymentID: testSpecDigest, TenantID: testTenant, Spec: h.svc.spec, Clock: h.clock})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.svc = restarted
	h.process(candidateVersion("sit-2", 1, 15))
	if items := scalar[int](h, "SELECT COUNT(*) FROM scheduler_items"); items != 2 {
		t.Fatalf("scheduler items = %d, want one per admitted trigger", items)
	}
	rows, err := h.db.QueryContext(t.Context(), "SELECT scheduler_item_id, trigger_id FROM scheduler_items")
	if err != nil {
		t.Fatalf("read scheduler items: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var itemID, triggerID string
		if err := rows.Scan(&itemID, &triggerID); err != nil {
			t.Fatalf("scan scheduler item: %v", err)
		}
		if itemID != domain.SchedulerItemID(triggerID) {
			t.Fatalf("scheduler item %s for trigger %s, want the id derived from the trigger", itemID, triggerID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate scheduler items: %v", err)
	}
}
