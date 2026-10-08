package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
)

// Decisions reads every Decision recorded for one episode, in order.
func (s Store) Decisions(ctx context.Context, episodeID string) ([]domain.DecisionView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT decision_id, COALESCE(attempt_id, ''), COALESCE(fence, 0), ordinal, situation_version, validation_status,
			COALESCE(rejection_reason, ''), decision_sha256, raw_json, created_at
		FROM decisions WHERE episode_id = ? ORDER BY ordinal`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("read episode decisions: %w", err)
	}
	return scanDecisionViews(rows)
}

func scanDecisionViews(rows *sql.Rows) ([]domain.DecisionView, error) {
	defer func() { _ = rows.Close() }()
	records := []domain.DecisionView{}
	for rows.Next() {
		var r domain.DecisionView
		var digest []byte
		if err := rows.Scan(&r.DecisionID, &r.AttemptID, &r.Fence, &r.Ordinal, &r.SituationVersion, &r.ValidationStatus, &r.RejectionReason, &digest, &r.Decision, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan episode decision: %w", err)
		}
		r.DecisionSHA256 = "sha256:" + hex.EncodeToString(digest)
		records = append(records, r)
	}
	return records, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}
