package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ReadEpisodeFence reads the episode's lifecycle, current attempt and fence;
// found is false for an unknown episode.
func (t *Tx) ReadEpisodeFence(ctx context.Context, episodeID string) (domain.EpisodeFence, bool, error) {
	var fence domain.EpisodeFence
	var attempt sql.NullString
	err := t.q.QueryRowContext(ctx, `SELECT lifecycle_status, current_attempt_id, current_fence FROM episodes WHERE episode_id = ?`, episodeID).Scan(&fence.Lifecycle, &attempt, &fence.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EpisodeFence{}, false, nil
	}
	if err != nil {
		return domain.EpisodeFence{}, false, fmt.Errorf("load episode identity: %w", err)
	}
	fence.Attempt, fence.HasAttempt = attempt.String, attempt.Valid
	return fence, true, nil
}

// ReadTerminalEpisodeFence reads the current attempt and fence only, for the
// acknowledgement of a cancellation on an already closed episode.
func (t *Tx) ReadTerminalEpisodeFence(ctx context.Context, episodeID string) (domain.EpisodeFence, bool, error) {
	var fence domain.EpisodeFence
	var attempt sql.NullString
	err := t.q.QueryRowContext(ctx, "SELECT current_attempt_id, current_fence FROM episodes WHERE episode_id = ?", episodeID).Scan(&attempt, &fence.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EpisodeFence{}, false, nil
	}
	if err != nil {
		return domain.EpisodeFence{}, false, fmt.Errorf("load terminal attempt identity: %w", err)
	}
	fence.Attempt, fence.HasAttempt = attempt.String, attempt.Valid
	return fence, true, nil
}

// ReadAttempt reads the attempt row under the identity; found is false when no
// attempt has that id, episode and fence.
func (t *Tx) ReadAttempt(ctx context.Context, identity domain.Identity) (domain.AttemptRecord, bool, error) {
	var record domain.AttemptRecord
	var owner sql.NullString
	err := t.q.QueryRowContext(ctx, "SELECT status, owner_epoch FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&record.Status, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptRecord{}, false, nil
	}
	if err != nil {
		return domain.AttemptRecord{}, false, fmt.Errorf("load attempt identity: %w", err)
	}
	record.OwnerEpoch, record.HasOwnerEpoch = owner.String, owner.Valid
	return record, true, nil
}

// ReadAttemptStatusOf reads the status of the attempt under the identity; a
// missing attempt is an error.
func (t *Tx) ReadAttemptStatusOf(ctx context.Context, identity domain.Identity) (domain.AttemptStatus, error) {
	var status domain.AttemptStatus
	if err := t.q.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&status); err != nil {
		return "", fmt.Errorf("load terminal attempt: %w", err)
	}
	return status, nil
}

// ReadAttemptStatus reads an attempt's status by id; a missing attempt is an error.
func (t *Tx) ReadAttemptStatus(ctx context.Context, attemptID string) (domain.AttemptStatus, error) {
	var status domain.AttemptStatus
	if err := t.q.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ?", attemptID).Scan(&status); err != nil {
		return "", fmt.Errorf("load attempt status: %w", err)
	}
	return status, nil
}

// ReadEpisodeAttemptStatus reads the status of an episode's attempt by id; a
// missing attempt is an error.
func (t *Tx) ReadEpisodeAttemptStatus(ctx context.Context, episodeID, attemptID string) (domain.AttemptStatus, error) {
	var status domain.AttemptStatus
	if err := t.q.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ?", attemptID, episodeID).Scan(&status); err != nil {
		return "", fmt.Errorf("load current attempt: %w", err)
	}
	return status, nil
}

// OwnerHoldsLease reports whether the epoch owns an unexpired runtime lease at
// nowText. The lease table belongs to control; this is a read of its row.
func (t *Tx) OwnerHoldsLease(ctx context.Context, epoch, nowText string) (bool, error) {
	_, held, err := storage.QueryOptional[string](ctx, t.q, `
		SELECT owner_epoch FROM runtime_owner
		WHERE singleton_id = 1 AND owner_epoch = ? AND lease_until > ?`, epoch, nowText)
	if err != nil {
		return false, fmt.Errorf("assert runtime owner epoch: %w", err)
	}
	return held, nil
}
