package replay

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// applyCapabilities runs the worker-aware part of a replay mode: recorded
// decisions are verified against the replay worklist, shadow executors are
// compared report-only, and counterfactual commands go only to the simulator.
func applyCapabilities(ctx context.Context, db *storage.DB, tenantID string, mode Mode, caps Capabilities, compiled *spec.CompiledSpec, evaluationTime time.Time, result *Result) error {
	episodes, err := loadReplayEpisodes(ctx, db, tenantID)
	if err != nil {
		return err
	}
	switch mode {
	case ModeRecorded:
		return applyRecorded(ctx, db, caps.RecordedLedger, episodes, result)
	case ModeShadow:
		return applyPairedShadow(ctx, db, tenantID, caps, compiled, episodes, evaluationTime, result)
	case ModeCounterfactual:
		return applyCounterfactual(ctx, caps, result)
	}
	return nil
}

func loadReplayEpisodes(ctx context.Context, db *storage.DB, tenantID string) ([]domain.ReplayEpisode, error) {
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
	return collectReplayEpisodes(rows)
}

func collectReplayEpisodes(rows *sql.Rows) ([]domain.ReplayEpisode, error) {
	var episodes []domain.ReplayEpisode
	for rows.Next() {
		var episode domain.ReplayEpisode
		var snapshotDigest []byte
		if err := rows.Scan(&episode.TriggerID, &episode.SituationID, &episode.SituationVersion, &episode.EpisodeID, &snapshotDigest); err != nil {
			return nil, fmt.Errorf("scan replay item: %w", err)
		}
		episode.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
		episodes = append(episodes, episode.Keyed())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay items: %w", err)
	}
	return episodes, nil
}
