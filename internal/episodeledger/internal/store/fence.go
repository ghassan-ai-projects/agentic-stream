package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// OwnerCheck is the runtime owner assertion an owned identity is fenced by.
type OwnerCheck = storage.OwnerCheck

// ReadEpisodeFence reads the episode's tenant, lifecycle, current attempt and
// fence; found is false for an unknown episode.
func (t *Tx) ReadEpisodeFence(ctx context.Context, episodeID string) (domain.EpisodeFence, bool, error) {
	var fence domain.EpisodeFence
	var attempt sql.NullString
	err := t.q.QueryRowContext(ctx, `SELECT tenant_id, lifecycle_status, current_attempt_id, current_fence FROM episodes WHERE episode_id = ?`, episodeID).Scan(&fence.TenantID, &fence.Lifecycle, &attempt, &fence.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EpisodeFence{}, false, nil
	}
	if err != nil {
		return domain.EpisodeFence{}, false, fmt.Errorf("load episode identity: %w", err)
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

// AssertOwner runs the runtime owner check for epoch on the caller's
// transaction. The lease table belongs to control, so the ledger neither reads
// nor interprets it; a missing check or transaction refuses the write.
func (t *Tx) AssertOwner(ctx context.Context, owner storage.OwnerCheck, epoch string) error {
	if owner == nil || t.tx == nil {
		return errOwnerUnchecked
	}
	if err := owner(ctx, t.tx, epoch); err != nil {
		return fmt.Errorf("assert runtime owner epoch %s: %w", epoch, err)
	}
	return nil
}

var errOwnerUnchecked = errors.New("an owned identity requires a runtime owner check on a transaction")
