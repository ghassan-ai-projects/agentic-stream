package episodeledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ValidateWorkerIdentity validates a worker identity against the current
// episode fence and attempt state. Snapshot equality is intentionally not part
// of this check.
func ValidateWorkerIdentity(ctx context.Context, tx *sql.Tx, identity Identity) error {
	if err := checkEpisodeFence(ctx, tx, identity); err != nil {
		return err
	}
	if identity.OwnerEpoch != "" {
		if err := assertRuntimeEpoch(ctx, tx, identity.OwnerEpoch, time.Now().UTC()); err != nil {
			return err
		}
	}
	return checkAttemptOpen(ctx, tx, identity)
}

// checkEpisodeFence requires an open episode whose current attempt and fence
// are exactly the identity's; an older fence is stale.
func checkEpisodeFence(ctx context.Context, tx *sql.Tx, identity Identity) error {
	state, err := readEpisodeFence(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	if state.lifecycle.closed() {
		return &IdentityError{Reason: RejectEpisodeClosed}
	}
	return state.checkIdentity(identity)
}

type episodeFence struct {
	lifecycle LifecycleStatus
	attempt   sql.NullString
	fence     int64
}

func readEpisodeFence(ctx context.Context, tx *sql.Tx, episodeID string) (episodeFence, error) {
	var state episodeFence
	err := tx.QueryRowContext(ctx, `SELECT lifecycle_status, current_attempt_id, current_fence FROM episodes WHERE episode_id = ?`, episodeID).Scan(&state.lifecycle, &state.attempt, &state.fence)
	if errors.Is(err, sql.ErrNoRows) {
		return state, &IdentityError{Reason: RejectUnknownEpisode}
	}
	if err != nil {
		return state, fmt.Errorf("load episode identity: %w", err)
	}
	return state, nil
}

func (state episodeFence) checkIdentity(identity Identity) error {
	if identity.Fence < state.fence {
		return &IdentityError{Reason: RejectStaleAttempt}
	}
	if !state.attempt.Valid || identity.AttemptID != state.attempt.String || identity.Fence != state.fence {
		return &IdentityError{Reason: RejectWrongAttempt}
	}
	return nil
}

// checkAttemptOpen requires the attempt row to exist under the identity's
// owner epoch and not be terminal.
func checkAttemptOpen(ctx context.Context, tx *sql.Tx, identity Identity) error {
	state, err := readAttemptIdentity(ctx, tx, identity)
	if err != nil {
		return err
	}
	if identity.OwnerEpoch != "" && (!state.ownerEpoch.Valid || state.ownerEpoch.String != identity.OwnerEpoch) {
		return &IdentityError{Reason: RejectStaleAttempt}
	}
	if IsTerminalAttempt(state.status) {
		return &IdentityError{Reason: RejectTerminalAttempt}
	}
	return nil
}

type attemptIdentity struct {
	status     AttemptStatus
	ownerEpoch sql.NullString
}

func readAttemptIdentity(ctx context.Context, tx *sql.Tx, identity Identity) (attemptIdentity, error) {
	var state attemptIdentity
	err := tx.QueryRowContext(ctx, "SELECT status, owner_epoch FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&state.status, &state.ownerEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		return state, &IdentityError{Reason: RejectWrongAttempt}
	}
	if err != nil {
		return state, fmt.Errorf("load attempt identity: %w", err)
	}
	return state, nil
}

// closed reports whether the episode lifecycle is terminal.
func (s LifecycleStatus) closed() bool {
	switch s {
	case LifecycleConcluded, LifecycleClosed, LifecycleSuperseded, LifecycleExpired, LifecycleAbandoned:
		return true
	default:
		return false
	}
}

func assertRuntimeEpoch(ctx context.Context, tx *sql.Tx, epoch string, now time.Time) error {
	var current string
	if err := tx.QueryRowContext(ctx, `
		SELECT owner_epoch FROM runtime_owner
		WHERE singleton_id = 1 AND owner_epoch = ? AND lease_until > ?`,
		epoch, formatTime(now),
	).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &IdentityError{Reason: RejectStaleAttempt}
		}
		return fmt.Errorf("assert runtime owner epoch: %w", err)
	}
	return nil
}

func validateTerminalAttemptIdentity(ctx context.Context, tx *sql.Tx, identity Identity) error {
	state, err := readTerminalEpisodeFence(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	if err := state.checkIdentity(identity); err != nil {
		return err
	}
	if identity.OwnerEpoch != "" {
		if err := assertRuntimeEpoch(ctx, tx, identity.OwnerEpoch, time.Now().UTC()); err != nil {
			return err
		}
	}
	return assertTerminalAttemptOpen(ctx, tx, identity)
}

func readTerminalEpisodeFence(ctx context.Context, tx *sql.Tx, episodeID string) (episodeFence, error) {
	var state episodeFence
	err := tx.QueryRowContext(ctx, "SELECT current_attempt_id, current_fence FROM episodes WHERE episode_id = ?", episodeID).Scan(&state.attempt, &state.fence)
	if errors.Is(err, sql.ErrNoRows) {
		return state, &IdentityError{Reason: RejectUnknownEpisode}
	}
	if err != nil {
		return state, fmt.Errorf("load terminal attempt identity: %w", err)
	}
	return state, nil
}

func assertTerminalAttemptOpen(ctx context.Context, tx *sql.Tx, identity Identity) error {
	var status AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&status); err != nil {
		return fmt.Errorf("load terminal attempt: %w", err)
	}
	if IsTerminalAttempt(status) {
		return &IdentityError{Reason: RejectTerminalAttempt}
	}
	return nil
}
