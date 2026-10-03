package episodeledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StartAttempt allocates the next fence for an admitted episode and records a
// dispatched worker attempt. It must be called inside the caller's transaction.
func StartAttempt(ctx context.Context, tx *sql.Tx, episodeID, attemptID string, now time.Time) (Identity, error) {
	return startAttempt(ctx, tx, episodeID, attemptID, "", now)
}

// StartAttemptOwned allocates an attempt fenced to the current runtime epoch.
// Live composition must use this entry point; StartAttempt remains available
// to isolated unit fixtures that do not model runtime ownership.
func StartAttemptOwned(ctx context.Context, tx *sql.Tx, episodeID, attemptID, ownerEpoch string, now time.Time) (Identity, error) {
	if ownerEpoch == "" {
		return Identity{}, fmt.Errorf("runtime owner epoch is required")
	}
	return startAttempt(ctx, tx, episodeID, attemptID, ownerEpoch, now)
}

func startAttempt(ctx context.Context, tx *sql.Tx, episodeID, attemptID, ownerEpoch string, now time.Time) (Identity, error) {
	if episodeID == "" || attemptID == "" {
		return Identity{}, fmt.Errorf("episode and attempt IDs are required")
	}
	if ownerEpoch != "" {
		if err := assertRuntimeEpoch(ctx, tx, ownerEpoch, now); err != nil {
			return Identity{}, err
		}
	}
	currentFence, err := requireStartableEpisode(ctx, tx, episodeID)
	if err != nil {
		return Identity{}, err
	}
	return recordStartedAttempt(ctx, tx, Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: currentFence + 1, OwnerEpoch: ownerEpoch}, now)
}

// requireStartableEpisode requires an admitted or running episode with no
// active attempt and returns its current fence.
func requireStartableEpisode(ctx context.Context, tx *sql.Tx, episodeID string) (int64, error) {
	state, err := readStartableEpisode(ctx, tx, episodeID)
	if err != nil {
		return 0, err
	}
	if state.lifecycle != LifecycleAdmitted && state.lifecycle != LifecycleRunning {
		return 0, &IdentityError{Reason: RejectEpisodeClosed}
	}
	if !state.attempt.Valid {
		return state.fence, nil
	}
	if err := requirePriorAttemptTerminal(ctx, tx, episodeID, state.attempt.String); err != nil {
		return 0, err
	}
	return state.fence, nil
}

func readStartableEpisode(ctx context.Context, tx *sql.Tx, episodeID string) (episodeFence, error) {
	var state episodeFence
	err := tx.QueryRowContext(ctx, loadStartableEpisodeSQL, episodeID).Scan(&state.lifecycle, &state.attempt, &state.fence)
	if errors.Is(err, sql.ErrNoRows) {
		return state, &IdentityError{Reason: RejectUnknownEpisode}
	}
	if err != nil {
		return state, fmt.Errorf("load episode for attempt: %w", err)
	}
	return state, nil
}

// insertAttempt records a dispatched attempt, owned by an epoch when one is
// given.
func insertAttempt(ctx context.Context, tx *sql.Tx, identity Identity, now time.Time) error {
	var err error
	if identity.OwnerEpoch == "" {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at)
			VALUES (?, ?, ?, ?, ?)`, identity.AttemptID, identity.EpisodeID, identity.Fence, AttemptDispatched, formatTime(now))
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at)
			VALUES (?, ?, ?, ?, ?, ?)`, identity.AttemptID, identity.EpisodeID, identity.Fence, AttemptDispatched, identity.OwnerEpoch, formatTime(now))
	}
	if err != nil {
		return fmt.Errorf("insert episode attempt: %w", err)
	}
	return nil
}

// TransitionAttempt applies a valid attempt transition and records terminal
// data. The identity is checked before the state mutation.
func TransitionAttempt(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus, now time.Time, terminalJSON []byte) error {
	if err := validateTransitionIdentity(ctx, tx, identity, to); err != nil {
		return err
	}
	if err := requireAttemptTransition(ctx, tx, identity, to); err != nil {
		return err
	}
	return persistAttemptTransition(ctx, tx, identity, to, now, terminalJSON)
}

func validateTransitionIdentity(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus) error {
	if err := ValidateWorkerIdentity(ctx, tx, identity); err != nil {
		// A superseded episode still needs to durably acknowledge cancellation
		// of the in-flight attempt. Do not allow a produced decision through
		// this exception; only cancellation/abandonment may close the attempt.
		if (to != AttemptCancelling && to != AttemptCancelled && to != AttemptAbandoned) || !IsIdentityReason(err, RejectEpisodeClosed) {
			return err
		}
		if terminalErr := validateTerminalAttemptIdentity(ctx, tx, identity); terminalErr != nil {
			return terminalErr
		}
	}
	return nil
}

func requireAttemptTransition(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus) error {
	var from AttemptStatus
	if err := tx.QueryRowContext(ctx,
		"SELECT status FROM episode_attempts WHERE attempt_id = ?",
		identity.AttemptID,
	).Scan(&from); err != nil {
		return fmt.Errorf("load attempt status: %w", err)
	}
	if !CanTransitionAttempt(from, to) {
		return fmt.Errorf("invalid attempt transition %s -> %s", from, to)
	}

	return nil
}

func persistAttemptTransition(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus, now time.Time, terminalJSON []byte) error {
	query, args := attemptTransitionSQL(identity, to, now, terminalJSON)
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("transition attempt: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("count transitioned attempts: %w", err)
	} else if n != 1 {
		return fmt.Errorf("attempt %s was not transitioned", identity.AttemptID)
	}
	return nil
}

func attemptTransitionSQL(identity Identity, to AttemptStatus, now time.Time, terminalJSON []byte) (string, []any) {
	var query string
	var args []any
	if IsTerminalAttempt(to) {
		query = `UPDATE episode_attempts SET status = ?, ended_at = ?, terminal_json = ? WHERE attempt_id = ? AND episode_id = ? AND fence = ?`
		args = []any{to, formatTime(now), terminalJSON, identity.AttemptID, identity.EpisodeID, identity.Fence}
	} else if to == AttemptRunning {
		query = `UPDATE episode_attempts SET status = ?, started_at = COALESCE(started_at, ?) WHERE attempt_id = ? AND episode_id = ? AND fence = ?`
		args = []any{to, formatTime(now), identity.AttemptID, identity.EpisodeID, identity.Fence}
	} else {
		query = `UPDATE episode_attempts SET status = ? WHERE attempt_id = ? AND episode_id = ? AND fence = ?`
		args = []any{to, identity.AttemptID, identity.EpisodeID, identity.Fence}
	}
	return query, args

}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func recordStartedAttempt(ctx context.Context, tx *sql.Tx, identity Identity, now time.Time) (Identity, error) {
	if err := insertAttempt(ctx, tx, identity, now); err != nil {
		return Identity{}, err
	}
	if _, err := tx.ExecContext(ctx, recordEpisodeAttemptSQL,
		LifecycleRunning, identity.AttemptID, identity.Fence, formatTime(now), identity.EpisodeID,
	); err != nil {
		return Identity{}, fmt.Errorf("update episode attempt identity: %w", err)
	}
	return identity, nil
}

func requirePriorAttemptTerminal(ctx context.Context, tx *sql.Tx, episodeID, attemptID string) error {
	var currentStatus AttemptStatus
	if err := tx.QueryRowContext(ctx,
		"SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ?",
		attemptID, episodeID,
	).Scan(&currentStatus); err != nil {
		return fmt.Errorf("load current attempt: %w", err)
	}
	if !IsTerminalAttempt(currentStatus) {
		return fmt.Errorf("episode %s already has active attempt %s", episodeID, attemptID)
	}
	return nil
}

const loadStartableEpisodeSQL = `
		SELECT lifecycle_status, current_attempt_id, current_fence
		FROM episodes WHERE episode_id = ?`

const recordEpisodeAttemptSQL = `
		UPDATE episodes
		SET lifecycle_status = ?, current_attempt_id = ?, current_fence = ?,
		    started_at = COALESCE(started_at, ?)
		WHERE episode_id = ?`
