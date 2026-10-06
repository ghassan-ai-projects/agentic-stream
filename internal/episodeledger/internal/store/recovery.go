package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
)

// UnfinishedAttempts lists the active attempts owned by an epoch other than
// the current one, closing the result set before returning.
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

// AbandonUnfinishedAttempt abandons one active attempt and returns the rows it changed.
func (t *Tx) AbandonUnfinishedAttempt(ctx context.Context, attemptID, episodeID, endedAt string, terminal []byte) (int64, error) {
	result, err := t.q.ExecContext(ctx, abandonAttemptSQL, endedAt, terminal, attemptID, episodeID)
	if err != nil {
		return 0, fmt.Errorf("abandon attempt %s: %w", attemptID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count abandoned attempt %s: %w", attemptID, err)
	}
	return count, nil
}

// AbandonOpenEpisode abandons an episode that is not yet closed and returns
// the rows it changed.
func (t *Tx) AbandonOpenEpisode(ctx context.Context, episodeID, endedAt string, terminal []byte) (int64, error) {
	result, err := t.q.ExecContext(ctx, abandonCancelingEpisodeSQL, endedAt, terminal, episodeID)
	if err != nil {
		return 0, fmt.Errorf("abandon canceling episode %s: %w", episodeID, err)
	}
	return rowsOrZero(result), nil
}

// EpisodeLifecycle reads the episode's lifecycle status.
func (t *Tx) EpisodeLifecycle(ctx context.Context, episodeID string) (domain.LifecycleStatus, error) {
	var lifecycle domain.LifecycleStatus
	if err := t.q.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err != nil {
		return "", fmt.Errorf("load recovered episode %s: %w", episodeID, err)
	}
	return lifecycle, nil
}

const unfinishedAttemptsSQL = `
		SELECT attempt_id, episode_id, status, COALESCE(owner_epoch, '')
		FROM episode_attempts
		WHERE status IN ('dispatched', 'running', 'cancelling')
		  AND (owner_epoch IS NULL OR owner_epoch <> ?)`

const abandonAttemptSQL = `
		UPDATE episode_attempts
		SET status = 'abandoned', ended_at = ?, terminal_json = ?
		WHERE attempt_id = ? AND episode_id = ?
		  AND status IN ('dispatched', 'running', 'cancelling')`

const abandonCancelingEpisodeSQL = `
		UPDATE episodes
		SET lifecycle_status = 'abandoned', ended_at = ?,
		    terminal_json = ?
		WHERE episode_id = ? AND lifecycle_status NOT IN
		    ('concluded', 'closed', 'superseded', 'expired', 'abandoned')`

// Settler settles or releases the cost reservation of an episode on the
// caller's transaction. The runtime's cost ledger satisfies it.
type Settler interface {
	Settle(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error
}

// SettleEpisodeCost releases the episode's cost reservation through the
// settler on this transaction.
func (t *Tx) SettleEpisodeCost(ctx context.Context, settler Settler, episodeID, now string) error {
	return settler.Settle(ctx, t.tx, episodeID, 0, now) //nolint:wrapcheck // The caller names the episode and operation.
}
