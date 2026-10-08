package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestInsertItemIgnoresDeterministicIDCollision(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	const (
		deploymentID = "dep-test"
		tenantID     = "tenant-test"
		situationID  = "sit-test"
	)
	knownItemID := "sch_0000000000000001"
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO spec_deployments (
				deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
				spec_sha256, source_json, compiled_ir, status, activated_at, created_at
			) VALUES (?, ?, 'test', '1', 'agentic-stream/v1', ?, ?, ?, 'active', ?, ?)`,
			deploymentID, tenantID, make([]byte, 32), []byte("{}"), []byte("{}"), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO lineage_sets (
				lineage_id, sha256, reference_count, references_json, created_at
			) VALUES ('lin-test', ?, 1, ?, ?)`, make([]byte, 32), []byte("[]"), kernel.FormatTime(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO situations (
				situation_id, tenant_id, deployment_id, situation_type, entity_type,
				entity_id, partition_id, occurrence_id, current_version, phase, status,
				first_event_time, latest_event_time, updated_at, created_at
			) VALUES (?, ?, ?, 'test', 'thing', 'thing-1', 0, 'occ-test', 1, 'candidate', 'active', ?, ?, ?, ?)`,
			situationID, tenantID, deploymentID, kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO situation_versions (
				situation_id, version, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
			) VALUES (?, 1, 'candidate', 10, 1.0, 'provisional', ?, ?, ?, ?, 'lin-test', ?)`,
			situationID, kernel.FormatTime(now), kernel.FormatTime(now), []byte("{}"), make([]byte, 32), kernel.FormatTime(now)); err != nil {
			return err
		}
		for _, triggerID := range []string{"trg-existing", "trg-new"} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO trigger_evaluations (
					trigger_id, tenant_id, deployment_id, trigger_name, situation_id,
					situation_version, score, threshold, lane, outcome, reasons_json,
					policy_sha256, evaluated_at
				) VALUES (?, ?, ?, 'test', ?, 1, 10, 5, 'fast', 'admitted', ?, ?, ?)`,
				triggerID, tenantID, deploymentID, situationID, []byte("[]"), make([]byte, 32), kernel.FormatTime(now)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
				kind, lane, priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES (?, 'trg-existing', ?, ?, 1, 'standard', 'fast', 10, 'pending', ?, ?, ?, ?)`,
			knownItemID, tenantID, situationID, make([]byte, 32), kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
			return err
		}

		scheduler := &scheduler{idGen: sources.Deterministic(), clk: sources.NewVirtual(now)}
		item := episodeledger.SchedulerItem{
			SchedulerItemID:  scheduler.itemID(),
			Kind:             "standard",
			TriggerID:        "trg-new",
			SituationID:      situationID,
			SituationVersion: 1,
			Lane:             "fast",
			Priority:         10,
			Status:           "pending",
			ExpiresAt:        now.Add(time.Hour),
		}
		if item.SchedulerItemID != knownItemID {
			t.Fatalf("deterministic item ID = %q, want %q", item.SchedulerItemID, knownItemID)
		}
		return scheduler.insertItem(ctx, store.Join(tx), item, tenantID)
	}); err != nil {
		t.Fatalf("insert item with existing scheduler item ID: %v", err)
	}

	var itemID, triggerID string
	if err := db.QueryRowContext(ctx, `
		SELECT scheduler_item_id, trigger_id FROM scheduler_items WHERE scheduler_item_id = ?`, knownItemID).
		Scan(&itemID, &triggerID); err != nil {
		t.Fatalf("query existing scheduler item: %v", err)
	}
	if itemID != knownItemID || triggerID != "trg-existing" {
		t.Fatalf("existing scheduler item = (%q, %q), want (%q, %q)", itemID, triggerID, knownItemID, "trg-existing")
	}
}
