package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// InsertAttempt records a dispatched attempt, owned by an epoch when the
// identity has one.
func (t *Tx) InsertAttempt(ctx context.Context, identity domain.Identity, startedAt time.Time) error {
	var err error
	if identity.OwnerEpoch == "" {
		_, err = t.q.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at)
			VALUES (?, ?, ?, ?, ?)`, identity.AttemptID, identity.EpisodeID, identity.Fence, domain.AttemptDispatched, kernel.FormatTime(startedAt))
	} else {
		_, err = t.q.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at)
			VALUES (?, ?, ?, ?, ?, ?)`, identity.AttemptID, identity.EpisodeID, identity.Fence, domain.AttemptDispatched, identity.OwnerEpoch, kernel.FormatTime(startedAt))
	}
	if err != nil {
		return fmt.Errorf("insert episode attempt: %w", err)
	}
	return nil
}

// RecordEpisodeAttempt marks the episode running under the new attempt and fence.
func (t *Tx) RecordEpisodeAttempt(ctx context.Context, identity domain.Identity, startedAt time.Time) error {
	if _, err := t.q.ExecContext(ctx, recordEpisodeAttemptSQL,
		domain.LifecycleRunning, identity.AttemptID, identity.Fence, kernel.FormatTime(startedAt), identity.EpisodeID,
	); err != nil {
		return fmt.Errorf("update episode attempt identity: %w", err)
	}
	return nil
}

// FinishAttempt moves the attempt to a terminal status with its terminal
// document and returns the rows it changed.
func (t *Tx) FinishAttempt(ctx context.Context, identity domain.Identity, to domain.AttemptStatus, endedAt time.Time, terminal []byte) (int64, error) {
	return t.updateAttempt(ctx, `UPDATE episode_attempts SET status = ?, ended_at = ?, terminal_json = ? WHERE attempt_id = ? AND episode_id = ? AND fence = ?`,
		to, kernel.FormatTime(endedAt), terminal, identity.AttemptID, identity.EpisodeID, identity.Fence)
}

// StartRunningAttempt moves the attempt to running, keeping its first start time.
func (t *Tx) StartRunningAttempt(ctx context.Context, identity domain.Identity, startedAt time.Time) (int64, error) {
	return t.updateAttempt(ctx, `UPDATE episode_attempts SET status = ?, started_at = COALESCE(started_at, ?) WHERE attempt_id = ? AND episode_id = ? AND fence = ?`,
		domain.AttemptRunning, kernel.FormatTime(startedAt), identity.AttemptID, identity.EpisodeID, identity.Fence)
}

// SetAttemptStatus changes only the attempt's status.
func (t *Tx) SetAttemptStatus(ctx context.Context, identity domain.Identity, to domain.AttemptStatus) (int64, error) {
	return t.updateAttempt(ctx, `UPDATE episode_attempts SET status = ? WHERE attempt_id = ? AND episode_id = ? AND fence = ?`,
		to, identity.AttemptID, identity.EpisodeID, identity.Fence)
}

func (t *Tx) updateAttempt(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := t.q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("transition attempt: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count transitioned attempts: %w", err)
	}
	return count, nil
}

const recordEpisodeAttemptSQL = `
		UPDATE episodes
		SET lifecycle_status = ?, current_attempt_id = ?, current_fence = ?,
		    started_at = COALESCE(started_at, ?)
		WHERE episode_id = ?`
