package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (tx *Tx) ApprovalSnapshotDigest(ctx context.Context, row domain.IntentRecord) ([]byte, error) {
	var digest []byte
	if err := tx.tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", row.SituationID, row.SituationVersion).Scan(&digest); err != nil {
		return nil, fmt.Errorf("load approval snapshot digest: %w", err)
	}
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("approval snapshot digest is incomplete")
	}
	return digest, nil
}

func (tx *Tx) ApprovalDelta(ctx context.Context, episodeID string) (map[string]any, error) {
	delta := map[string]any{}
	var raw []byte
	err := tx.tx.QueryRowContext(ctx, loadApprovalDeltaSQL, episodeID).Scan(&raw)
	if err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &delta); err != nil {
			return nil, fmt.Errorf("decode approval delta: %w", err)
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load approval delta: %w", err)
	}
	if delta == nil {
		return nil, fmt.Errorf("approval delta must be an object")
	}
	return delta, nil
}

const loadApprovalDeltaSQL = `
		SELECT te.delta_json
		FROM trigger_evaluations te
		JOIN scheduler_items si ON si.trigger_id = te.trigger_id
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		WHERE e.episode_id = ?
		ORDER BY te.evaluated_at DESC LIMIT 1`
