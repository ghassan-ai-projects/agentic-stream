package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const unansweredSQL = `WITH ranked AS (
	SELECT e.situation_id, e.situation_version, e.lifecycle_status,
		EXISTS (SELECT 1 FROM decisions d WHERE d.episode_id = e.episode_id AND d.validation_status = 'accepted') AS answered,
		ROW_NUMBER() OVER (PARTITION BY e.situation_id ORDER BY e.accepted_at DESC, e.episode_id DESC) AS recency
	FROM episodes e WHERE e.tenant_id = ?)
SELECT r.situation_id, r.situation_version FROM ranked r
WHERE r.lifecycle_status = 'concluded' AND NOT r.answered
	AND EXISTS (SELECT 1 FROM ranked latest WHERE latest.situation_id = r.situation_id AND latest.recency = 1
		AND latest.lifecycle_status = 'concluded' AND NOT latest.answered)
ORDER BY r.situation_id, r.situation_version`

func (t *Tx) UnansweredReasoning(ctx context.Context, tenantID string) ([]domain.Unanswered, error) {
	unanswered, err := storage.QueryAll(ctx, t.q, "unanswered reasoning", scanUnanswered, unansweredSQL, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read unanswered reasoning: %w", err)
	}
	return unanswered, nil
}

func scanUnanswered(rows *sql.Rows) (domain.Unanswered, error) {
	var row domain.Unanswered
	if err := rows.Scan(&row.SituationID, &row.SituationVersion); err != nil {
		return row, fmt.Errorf("scan unanswered reasoning: %w", err)
	}
	return row, nil
}
