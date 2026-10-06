package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// RecordEpochState records a drain or kill. Kill is terminal: once an epoch is
// killed, neither a later drain nor a repeated kill rewrites the row.
func (t *Tx) RecordEpochState(ctx context.Context, epoch, state, nowText string) error {
	if _, err := t.q.ExecContext(ctx, epochControlUpsert, epoch, state, nowText); err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}

// EpochState reads the epoch's control state; what names the read in errors.
func (t *Tx) EpochState(ctx context.Context, epoch, what string) (state string, found bool, err error) {
	err = t.q.QueryRowContext(ctx, `SELECT state FROM epoch_control WHERE epoch = ?`, epoch).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", what, err)
	}
	return state, true, nil
}

// SupersedeEpoch cancels the epoch's in-flight episodes through the episode
// ledger on this transaction.
func (t *Tx) SupersedeEpoch(ctx context.Context, epoch, nowText string) error {
	if err := episodeledger.SupersedeEpoch(ctx, t.tx, epoch, nowText); err != nil {
		return fmt.Errorf("supersede epoch episodes: %w", err)
	}
	return nil
}

// UnstartedReservedEpisodes lists admitted episodes of the epoch that hold a
// cost reservation but never started an attempt. The episode table belongs to
// the episode ledger; the read is joined with this module's reservations.
func (t *Tx) UnstartedReservedEpisodes(ctx context.Context, epoch string) ([]string, error) {
	rows, err := t.q.QueryContext(ctx, unstartedEpisodeReservationsSQL, epoch)
	if err != nil {
		return nil, fmt.Errorf("list admitted epoch reservations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	episodeIDs, err := collectEpisodeIDs(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close admitted epoch reservations: %w", err)
	}
	return episodeIDs, nil
}

func collectEpisodeIDs(rows *sql.Rows) ([]string, error) {
	var episodeIDs []string
	for rows.Next() {
		var episodeID string
		if err := rows.Scan(&episodeID); err != nil {
			return nil, fmt.Errorf("scan admitted epoch reservation: %w", err)
		}
		episodeIDs = append(episodeIDs, episodeID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read admitted epoch reservations: %w", err)
	}
	return episodeIDs, nil
}

const epochControlUpsert = `
		INSERT INTO epoch_control (epoch, state, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
		WHERE epoch_control.state <> 'killed'`

const unstartedEpisodeReservationsSQL = `
		SELECT e.episode_id
		FROM episodes e JOIN cost_reservations r ON r.episode_id = e.episode_id
		WHERE e.policy_epoch = ? AND e.lifecycle_status = 'admitted'
		  AND e.current_attempt_id IS NULL AND r.status = 'reserved'`
