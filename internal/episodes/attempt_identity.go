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
		RejectOversized, RejectExpired, RejectIntentTypeNotAllowed, RejectRiskCeilingExceeded,
		RejectCatalogMissing, RejectCatalogForged, RejectIntentTypeNotInCatalog, RejectRiskLabelMismatch,
		RejectParameterSchemaViolated, RejectPresetMismatch, RejectUngroundedEvidence:
		return true
	default:
		return false
	}
}
