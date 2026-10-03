package replay

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type replayItem struct {
	TriggerID        string
	SituationID      string
	SituationVersion int
	EpisodeID        string
	SnapshotDigest   string
}

// applyCapabilities runs the worker-aware part of a replay mode: recorded
// decisions are verified against the replay worklist, shadow executors are
// compared report-only, and counterfactual commands go only to the simulator.
func applyCapabilities(ctx context.Context, db *storage.DB, tenantID string, mode Mode, caps Capabilities, compiled *spec.CompiledSpec, evaluationTime time.Time, result *Result) error {
	items, err := loadReplayItems(ctx, db, tenantID)
	if err != nil {
		return err
	}
	switch mode {
	case ModeRecorded:
		return applyRecorded(ctx, db, caps.RecordedLedger, items, result)
	case ModeShadow:
		return applyPairedShadow(ctx, db, tenantID, caps, compiled, items, evaluationTime, result)
	case ModeCounterfactual:
		return applyCounterfactual(ctx, caps, result)
	}
	return nil
}

func loadReplayItems(ctx context.Context, db *storage.DB, tenantID string) ([]replayItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT si.trigger_id, si.situation_id, si.situation_version, e.episode_id, sv.snapshot_sha256
		FROM scheduler_items si
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		JOIN situation_versions sv ON sv.situation_id = si.situation_id AND sv.version = si.situation_version
		WHERE si.tenant_id = ?
		ORDER BY si.situation_id, si.situation_version, si.trigger_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query replay items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectReplayItems(rows)
}

func collectReplayItems(rows *sql.Rows) ([]replayItem, error) {
	var items []replayItem
	for rows.Next() {
		var item replayItem
		var snapshotDigest []byte
		if err := rows.Scan(&item.TriggerID, &item.SituationID, &item.SituationVersion, &item.EpisodeID, &snapshotDigest); err != nil {
			return nil, fmt.Errorf("scan replay item: %w", err)
		}
		item.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay items: %w", err)
	}
	return items, nil
}
