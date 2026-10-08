package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
)

func (s Store) Decisions(ctx context.Context, episodeID string) ([]domain.DecisionView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT decision_id, COALESCE(attempt_id, ''), COALESCE(fence, 0), ordinal, situation_version, validation_status,
			COALESCE(rejection_reason, ''), decision_sha256, raw_json, created_at
		FROM decisions WHERE episode_id = ? ORDER BY ordinal`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("read episode decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	decisions, err := storage.CollectRows(rows, "episode decisions", scanDecisionView)
	if err != nil {
		return nil, fmt.Errorf("read episode decisions: %w", err)
	}
	return decisions, nil
}

func scanDecisionView(rows *sql.Rows) (domain.DecisionView, error) {
	var r domain.DecisionView
	var digest []byte
	if err := rows.Scan(&r.DecisionID, &r.AttemptID, &r.Fence, &r.Ordinal, &r.SituationVersion, &r.ValidationStatus, &r.RejectionReason, &digest, &r.Decision, &r.CreatedAt); err != nil {
		return domain.DecisionView{}, fmt.Errorf("scan episode decision: %w", err)
	}
	r.DecisionSHA256 = "sha256:" + hex.EncodeToString(digest)
	return r, nil
}
