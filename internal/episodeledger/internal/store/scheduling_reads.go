package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
)

const schedulingSQL = `
	SELECT si.scheduler_item_id, si.status, si.lane, si.priority, si.expires_at, si.updated_at,
		COALESCE(e.episode_id, ''), COALESCE(e.lifecycle_status, ''), COALESCE(e.situation_version, 0)
	FROM scheduler_items si
	LEFT JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
	WHERE si.tenant_id = ? AND si.trigger_id = ?`

// Scheduling reads what became of one trigger evaluation; found is false
// when the evaluation never created a scheduler item.
func (t *Tx) Scheduling(ctx context.Context, tenantID, triggerID string) (domain.SchedulingRecord, bool, error) {
	var record domain.SchedulingRecord
	err := t.q.QueryRowContext(ctx, schedulingSQL, tenantID, triggerID).Scan(&record.SchedulerItemID, &record.Status, &record.Lane, &record.Priority, &record.ExpiresAt, &record.UpdatedAt,
		&record.EpisodeID, &record.EpisodeStatus, &record.EpisodeSituationVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SchedulingRecord{}, false, nil
	}
	if err != nil {
		return domain.SchedulingRecord{}, false, fmt.Errorf("read scheduling of trigger %s: %w", triggerID, err)
	}
	record.Rejections, err = t.Rejections(ctx, record.EpisodeID)
	return record, err == nil, err
}

// Rejections reads the results the ledger refused for one episode, oldest
// first.
func (t *Tx) Rejections(ctx context.Context, episodeID string) ([]domain.RejectionRecord, error) {
	rows, err := t.q.QueryContext(ctx, `
		SELECT reason, COALESCE(attempt_id, ''), fence, details_json, created_at
		FROM episode_rejections WHERE episode_id = ? ORDER BY created_at, rejection_id`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("read episode rejections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	rejections := []domain.RejectionRecord{}
	for rows.Next() {
		var rejection domain.RejectionRecord
		if err := rows.Scan(&rejection.Reason, &rejection.AttemptID, &rejection.Fence, &rejection.Details, &rejection.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan episode rejection: %w", err)
		}
		rejections = append(rejections, rejection)
	}
	return rejections, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}
