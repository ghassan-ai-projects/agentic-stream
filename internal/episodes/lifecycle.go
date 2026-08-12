package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// LifecycleStatus is the coordination state of an episode aggregate. It does
// not describe a Decision, intent, command, or outcome.
type LifecycleStatus string

const (
	LifecycleAdmitted   LifecycleStatus = "admitted"
	LifecycleRunning    LifecycleStatus = "running"
	LifecycleConcluded  LifecycleStatus = "concluded"
	LifecycleClosed     LifecycleStatus = "closed"
	LifecycleSuperseded LifecycleStatus = "superseded"
	LifecycleExpired    LifecycleStatus = "expired"
	LifecycleAbandoned  LifecycleStatus = "abandoned"
)

// AttemptStatus is the terminal state of one worker dispatch.
type AttemptStatus string

const (
	AttemptDispatched AttemptStatus = "dispatched"
	AttemptRunning    AttemptStatus = "running"
	AttemptCancelling AttemptStatus = "cancelling" //nolint:misspell // Frozen durable protocol value.
	AttemptProduced   AttemptStatus = "produced"
	AttemptDeclined   AttemptStatus = "declined"
	AttemptCancelled  AttemptStatus = "cancelled" //nolint:misspell // Frozen durable protocol value.
	AttemptFailed     AttemptStatus = "failed"
	AttemptTimedOut   AttemptStatus = "timed_out"
	AttemptAbandoned  AttemptStatus = "abandoned"
)

// RejectionReason is a durable reason for refusing worker input or a
// proposed Decision.
type RejectionReason string

const (
	RejectUnknownEpisode       RejectionReason = "unknown_episode"
	RejectStaleAttempt         RejectionReason = "stale_attempt"
	RejectWrongAttempt         RejectionReason = "wrong_attempt"
	RejectTerminalAttempt      RejectionReason = "terminal_attempt"
	RejectEpisodeClosed        RejectionReason = "episode_closed"
	RejectSchemaInvalid        RejectionReason = "schema_invalid"
	RejectSnapshotMismatch     RejectionReason = "snapshot_mismatch"
	RejectEvidenceNotVisible   RejectionReason = "evidence_not_visible"
	RejectForgedReference      RejectionReason = "forged_reference"
	RejectOversized            RejectionReason = "oversized"
	RejectExpired              RejectionReason = "expired"
	RejectIntentTypeNotAllowed RejectionReason = "intent_type_not_allowed"
	RejectRiskCeilingExceeded  RejectionReason = "risk_ceiling_exceeded"
)

// Identity is the fencing identity carried by every worker-produced object.
type Identity struct {
	EpisodeID  string
	AttemptID  string
	Fence      int64
	OwnerEpoch string
}

// IdentityError identifies why worker input was refused.
type IdentityError struct {
	Reason RejectionReason
}

func (e *IdentityError) Error() string {
	return fmt.Sprintf("worker identity rejected: %s", e.Reason)
}

// IsIdentityReason reports whether err is a fencing rejection with reason.
func IsIdentityReason(err error, reason RejectionReason) bool {
	var identityErr *IdentityError
	return errors.As(err, &identityErr) && identityErr.Reason == reason
}

// CanTransitionAttempt reports whether an attempt state transition is valid.
func CanTransitionAttempt(from, to AttemptStatus) bool {
	switch from {
	case AttemptDispatched:
		return to == AttemptRunning || to == AttemptCancelling || to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	case AttemptRunning:
		return to == AttemptCancelling || to == AttemptProduced || to == AttemptDeclined || to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	case AttemptCancelling:
		return to == AttemptCancelled || to == AttemptFailed || to == AttemptTimedOut || to == AttemptAbandoned
	default:
		return false
	}
}

// IsTerminalAttempt reports whether an attempt state is terminal.
func IsTerminalAttempt(status AttemptStatus) bool {
	switch status {
	case AttemptProduced, AttemptDeclined, AttemptCancelled, AttemptFailed, AttemptTimedOut, AttemptAbandoned:
		return true
	default:
		return false
	}
}

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

	var lifecycle LifecycleStatus
	var currentAttempt sql.NullString
	var currentFence int64
	if err := tx.QueryRowContext(ctx, `
		SELECT lifecycle_status, current_attempt_id, current_fence
		FROM episodes WHERE episode_id = ?`, episodeID,
	).Scan(&lifecycle, &currentAttempt, &currentFence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Identity{}, &IdentityError{Reason: RejectUnknownEpisode}
		}
		return Identity{}, fmt.Errorf("load episode for attempt: %w", err)
	}
	if lifecycle != LifecycleAdmitted && lifecycle != LifecycleRunning {
		return Identity{}, &IdentityError{Reason: RejectEpisodeClosed}
	}
	if currentAttempt.Valid {
		var currentStatus AttemptStatus
		if err := tx.QueryRowContext(ctx,
			"SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ?",
			currentAttempt.String, episodeID,
		).Scan(&currentStatus); err != nil {
			return Identity{}, fmt.Errorf("load current attempt: %w", err)
		}
		if !IsTerminalAttempt(currentStatus) {
			return Identity{}, fmt.Errorf("episode %s already has active attempt %s", episodeID, currentAttempt.String)
		}
	}

	fence := currentFence + 1
	var insertErr error
	if ownerEpoch == "" {
		_, insertErr = tx.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at)
			VALUES (?, ?, ?, ?, ?)`, attemptID, episodeID, fence, AttemptDispatched, formatTime(now))
	} else {
		_, insertErr = tx.ExecContext(ctx, `
			INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at)
			VALUES (?, ?, ?, ?, ?, ?)`, attemptID, episodeID, fence, AttemptDispatched, ownerEpoch, formatTime(now))
	}
	if insertErr != nil {
		return Identity{}, fmt.Errorf("insert episode attempt: %w", insertErr)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes
		SET lifecycle_status = ?, current_attempt_id = ?, current_fence = ?,
		    started_at = COALESCE(started_at, ?)
		WHERE episode_id = ?`,
		LifecycleRunning, attemptID, fence, formatTime(now), episodeID,
	); err != nil {
		return Identity{}, fmt.Errorf("update episode attempt identity: %w", err)
	}
	return Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: fence, OwnerEpoch: ownerEpoch}, nil
}

// ValidateWorkerIdentity validates a worker identity against the current
// episode fence and attempt state. Snapshot equality is intentionally not part
// of this check.
func ValidateWorkerIdentity(ctx context.Context, tx *sql.Tx, identity Identity) error {
	var lifecycle LifecycleStatus
	var currentAttempt sql.NullString
	var currentFence int64
	if err := tx.QueryRowContext(ctx, `
		SELECT lifecycle_status, current_attempt_id, current_fence
		FROM episodes WHERE episode_id = ?`, identity.EpisodeID,
	).Scan(&lifecycle, &currentAttempt, &currentFence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &IdentityError{Reason: RejectUnknownEpisode}
		}
		return fmt.Errorf("load episode identity: %w", err)
	}
	if lifecycle == LifecycleConcluded || lifecycle == LifecycleClosed || lifecycle == LifecycleSuperseded || lifecycle == LifecycleExpired || lifecycle == LifecycleAbandoned {
		return &IdentityError{Reason: RejectEpisodeClosed}
	}
	if identity.Fence < currentFence {
		return &IdentityError{Reason: RejectStaleAttempt}
	}
	if !currentAttempt.Valid || identity.AttemptID != currentAttempt.String || identity.Fence != currentFence {
		return &IdentityError{Reason: RejectWrongAttempt}
	}
	if identity.OwnerEpoch != "" {
		if err := assertRuntimeEpoch(ctx, tx, identity.OwnerEpoch, time.Now().UTC()); err != nil {
			return err
		}
	}

	var status AttemptStatus
	var ownerEpoch sql.NullString
	if err := tx.QueryRowContext(ctx,
		"SELECT status, owner_epoch FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?",
		identity.AttemptID, identity.EpisodeID, identity.Fence,
	).Scan(&status, &ownerEpoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &IdentityError{Reason: RejectWrongAttempt}
		}
		return fmt.Errorf("load attempt identity: %w", err)
	}
	if identity.OwnerEpoch != "" && (!ownerEpoch.Valid || ownerEpoch.String != identity.OwnerEpoch) {
		return &IdentityError{Reason: RejectStaleAttempt}
	}
	if IsTerminalAttempt(status) {
		return &IdentityError{Reason: RejectTerminalAttempt}
	}
	return nil
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

// TransitionAttempt applies a valid attempt transition and records terminal
// data. The identity is checked before the state mutation.
func TransitionAttempt(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus, now time.Time, terminalJSON []byte) error {
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

func validateTerminalAttemptIdentity(ctx context.Context, tx *sql.Tx, identity Identity) error {
	var currentAttempt sql.NullString
	var currentFence int64
	if err := tx.QueryRowContext(ctx, "SELECT current_attempt_id, current_fence FROM episodes WHERE episode_id = ?", identity.EpisodeID).Scan(&currentAttempt, &currentFence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &IdentityError{Reason: RejectUnknownEpisode}
		}
		return fmt.Errorf("load terminal attempt identity: %w", err)
	}
	if identity.Fence < currentFence {
		return &IdentityError{Reason: RejectStaleAttempt}
	}
	if !currentAttempt.Valid || identity.AttemptID != currentAttempt.String || identity.Fence != currentFence {
		return &IdentityError{Reason: RejectWrongAttempt}
	}
	if identity.OwnerEpoch != "" {
		if err := assertRuntimeEpoch(ctx, tx, identity.OwnerEpoch, time.Now().UTC()); err != nil {
			return err
		}
	}
	var status AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&status); err != nil {
		return fmt.Errorf("load terminal attempt: %w", err)
	}
	if IsTerminalAttempt(status) {
		return &IdentityError{Reason: RejectTerminalAttempt}
	}
	return nil
}

// RecordRejection durably records a rejected worker input or Decision. It is
// idempotent for the same identity, reason, details, and timestamp.
func RecordRejection(ctx context.Context, tx *sql.Tx, identity Identity, reason RejectionReason, detailsJSON []byte, now time.Time) error {
	if !validRejectionReason(reason) {
		return fmt.Errorf("invalid rejection reason %q", reason)
	}
	if len(detailsJSON) == 0 {
		detailsJSON = []byte(`{}`)
	}
	material := fmt.Sprintf("%s|%s|%d|%s|%s|%s", identity.EpisodeID, identity.AttemptID, identity.Fence, reason, string(detailsJSON), formatTime(now))
	hash := sha256.Sum256([]byte(material))
	rejectionID := "rej_" + hex.EncodeToString(hash[:])
	var episode any
	if identity.EpisodeID != "" {
		var exists int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM episodes WHERE episode_id = ?", identity.EpisodeID).Scan(&exists)
		switch {
		case err == nil:
			episode = identity.EpisodeID
		case errors.Is(err, sql.ErrNoRows):
			// Unknown episodes must still be recorded. Keep the nullable foreign
			// key empty rather than allowing the rejection audit to fail.
		case err != nil:
			return fmt.Errorf("check rejected episode: %w", err)
		}
	}
	var attempt any
	if identity.AttemptID != "" {
		attempt = identity.AttemptID
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO episode_rejections (
			rejection_id, episode_id, attempt_id, fence, reason, details_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (rejection_id) DO NOTHING`,
		rejectionID, episode, attempt, identity.Fence, reason, detailsJSON, formatTime(now),
	)
	if err != nil {
		return fmt.Errorf("record rejection: %w", err)
	}
	return nil
}

func validRejectionReason(reason RejectionReason) bool {
	switch reason {
	case RejectUnknownEpisode, RejectStaleAttempt, RejectWrongAttempt, RejectTerminalAttempt, RejectEpisodeClosed,
		RejectSchemaInvalid, RejectSnapshotMismatch, RejectEvidenceNotVisible, RejectForgedReference,
		RejectOversized, RejectExpired, RejectIntentTypeNotAllowed, RejectRiskCeilingExceeded:
		return true
	default:
		return false
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
