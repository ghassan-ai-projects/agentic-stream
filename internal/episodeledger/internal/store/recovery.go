package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) UnfinishedAttempts(ctx context.Context, currentEpoch string) ([]domain.UnfinishedAttempt, error) {
	rows, err := t.q.QueryContext(ctx, unfinishedAttemptsSQL, currentEpoch)
	if err != nil {
		return nil, fmt.Errorf("list unfinished attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	attempts, err := collectUnfinishedAttempts(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close unfinished attempts: %w", err)
	}
	return attempts, nil
}

func collectUnfinishedAttempts(rows *sql.Rows) ([]domain.UnfinishedAttempt, error) {
	var attempts []domain.UnfinishedAttempt
	for rows.Next() {
		var item domain.UnfinishedAttempt
		if err := rows.Scan(&item.AttemptID, &item.EpisodeID, &item.Status, &item.OwnerEpoch); err != nil {
			return nil, fmt.Errorf("scan unfinished attempt: %w", err)
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unfinished attempts: %w", err)
	}
	return attempts, nil
}

func (t *Tx) AbandonUnfinishedAttempt(ctx context.Context, attemptID, episodeID string, endedAt time.Time, terminal []byte) (int64, error) {
	result, err := t.q.ExecContext(ctx, abandonAttemptSQL, kernel.FormatTime(endedAt), terminal, attemptID, episodeID)
	if err != nil {
		return 0, fmt.Errorf("abandon attempt %s: %w", attemptID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count abandoned attempt %s: %w", attemptID, err)
	}
	return count, nil
}

func (t *Tx) AbandonOpenEpisode(ctx context.Context, episodeID string, endedAt time.Time, terminal []byte) (int64, error) {
	result, err := t.q.ExecContext(ctx, abandonCancelingEpisodeSQL, kernel.FormatTime(endedAt), terminal, episodeID)
	if err != nil {
		return 0, fmt.Errorf("abandon canceling episode %s: %w", episodeID, err)
	}
	changed, err := storage.RowsAffected(result)
	if err != nil {
		return 0, fmt.Errorf("abandon canceling episode %s: %w", episodeID, err)
	}
	return changed, nil
}

func (t *Tx) EpisodeLifecycle(ctx context.Context, episodeID string) (domain.LifecycleStatus, error) {
	fence, found, err := t.ReadEpisodeFence(ctx, episodeID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("load episode lifecycle %s: %w", episodeID, sql.ErrNoRows)
	}
	return fence.Lifecycle, nil
}

var unfinishedAttemptsSQL = `
		SELECT attempt_id, episode_id, status, COALESCE(owner_epoch, '')
		FROM episode_attempts
		WHERE status IN ` + unfinishedAttempts + `
		  AND (owner_epoch IS NULL OR owner_epoch <> ?)`

var abandonAttemptSQL = `
		UPDATE episode_attempts
		SET status = 'abandoned', ended_at = ?, terminal_json = ?
		WHERE attempt_id = ? AND episode_id = ?
		  AND status IN ` + unfinishedAttempts

var abandonCancelingEpisodeSQL = `
		UPDATE episodes
		SET lifecycle_status = 'abandoned', ended_at = ?,
		    terminal_json = ?
		WHERE episode_id = ? AND lifecycle_status NOT IN ` + closedLifecycles

type Settler interface {
	Settle(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error
}

func (t *Tx) SettleEpisodeCost(ctx context.Context, settler Settler, episodeID string, now time.Time) error {
	if err := settler.Settle(ctx, t.tx, episodeID, 0, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("settle cost of episode %s: %w", episodeID, err)
	}
	return nil
}
