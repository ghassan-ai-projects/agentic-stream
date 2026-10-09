package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestInsertItemKeepsTheExistingItemOnADeterministicIDCollision(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	existing := scalar[string](h, "SELECT scheduler_item_id FROM scheduler_items")
	h.exec(`INSERT INTO trigger_evaluations (
		trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome,
		reasons_json, policy_sha256, evaluated_at
	) VALUES ('trg-new', ?, ?, 'other', 'sit-1', 1, 10, 5, 'fast', 'admitted', X'5B5D', zeroblob(32), ?)`, testTenant, testSpecDigest, kernel.FormatTime(base))
	colliding := &scheduler{idGen: sources.Deterministic(), clk: h.clock}
	item := episodeledger.SchedulerItem{
		SchedulerItemID: colliding.itemID(), Kind: episodeledger.KindStandard, TriggerID: "trg-new", SituationID: "sit-1",
		SituationVersion: 1, Lane: "fast", Priority: 10, Status: "pending", ExpiresAt: base.Add(15 * time.Minute),
	}
	if item.SchedulerItemID != existing {
		t.Fatalf("deterministic item id = %q, want the collision with %q", item.SchedulerItemID, existing)
	}
	if err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error { return colliding.insertItem(t.Context(), store.Join(tx), item, testTenant) }); err != nil {
		t.Fatalf("insertItem on a colliding id: %v", err)
	}
	if got := scalar[string](h, "SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?", existing); got == "trg-new" {
		t.Fatalf("the colliding insert replaced the existing item: trigger %s", got)
	}
	if rows := scalar[int](h, "SELECT COUNT(*) FROM scheduler_items"); rows != 1 {
		t.Fatalf("scheduler items = %d, want 1", rows)
	}
}
