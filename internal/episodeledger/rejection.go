package episodeledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// RecordRejection durably records a rejected worker input or Decision. It is
// idempotent for the same identity, reason, details, and timestamp.
func RecordRejection(ctx context.Context, tx *sql.Tx, identity Identity, reason RejectionReason, detailsJSON []byte, now time.Time) error {
	if !validRejectionReason(reason) {
		return fmt.Errorf("invalid rejection reason %q", reason)
	}
	if len(detailsJSON) == 0 {
		detailsJSON = []byte(`{}`)
	}
	rejectionID := workerRejectionID(identity, reason, detailsJSON, now)
	episode, err := rejectedEpisode(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	return insertRejection(ctx, tx, identity, rejectionID, episode, reason, detailsJSON, now)
}

func workerRejectionID(identity Identity, reason RejectionReason, detailsJSON []byte, now time.Time) string {
	material := fmt.Sprintf("%s|%s|%d|%s|%s|%s", identity.EpisodeID, identity.AttemptID, identity.Fence, reason, string(detailsJSON), formatTime(now))
	hash := sha256.Sum256([]byte(material))
	return "rej_" + hex.EncodeToString(hash[:])
}

func rejectedEpisode(ctx context.Context, tx *sql.Tx, episodeID string) (any, error) {
	if episodeID == "" {
		return nil, nil
	}
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM episodes WHERE episode_id = ?", episodeID).Scan(&exists)
	if err == nil {
		return episodeID, nil
	}
	// Unknown episodes still produce a rejection audit with a nullable foreign key.
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return nil, fmt.Errorf("check rejected episode: %w", err)
}

func insertRejection(ctx context.Context, tx *sql.Tx, identity Identity, rejectionID string, episode any, reason RejectionReason, detailsJSON []byte, now time.Time) error {
	var attempt any
	if identity.AttemptID != "" {
		attempt = identity.AttemptID
	}
	_, err := tx.ExecContext(ctx, insertWorkerRejectionSQL, rejectionID, episode, attempt, identity.Fence, reason, detailsJSON, formatTime(now))
	if err != nil {
		return fmt.Errorf("record rejection: %w", err)
	}
	return nil
}

const insertWorkerRejectionSQL = `
 INSERT INTO episode_rejections (
 rejection_id, episode_id, attempt_id, fence, reason, details_json, created_at
 ) VALUES (?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (rejection_id) DO NOTHING`

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
