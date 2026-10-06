package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// RejectionReason is a durable reason for refusing worker input or a
// proposed Decision.
type RejectionReason string

// Rejection reasons. The catalog reasons ride the same durable registry:
// the intent catalog is the independently verified authority.
const (
	RejectUnknownEpisode          RejectionReason = "unknown_episode"
	RejectStaleAttempt            RejectionReason = "stale_attempt"
	RejectWrongAttempt            RejectionReason = "wrong_attempt"
	RejectTerminalAttempt         RejectionReason = "terminal_attempt"
	RejectEpisodeClosed           RejectionReason = "episode_closed"
	RejectSchemaInvalid           RejectionReason = "schema_invalid"
	RejectSnapshotMismatch        RejectionReason = "snapshot_mismatch"
	RejectEvidenceNotVisible      RejectionReason = "evidence_not_visible"
	RejectForgedReference         RejectionReason = "forged_reference"
	RejectOversized               RejectionReason = "oversized"
	RejectExpired                 RejectionReason = "expired"
	RejectIntentTypeNotAllowed    RejectionReason = "intent_type_not_allowed"
	RejectRiskCeilingExceeded     RejectionReason = "risk_ceiling_exceeded"
	RejectCatalogMissing          RejectionReason = "catalog_missing"
	RejectCatalogForged           RejectionReason = "catalog_forged"
	RejectIntentTypeNotInCatalog  RejectionReason = "intent_type_not_in_catalog"
	RejectRiskLabelMismatch       RejectionReason = "risk_label_mismatch"
	RejectParameterSchemaViolated RejectionReason = "parameter_schema_violation"
	RejectPresetMismatch          RejectionReason = "preset_mismatch"
	RejectUngroundedEvidence      RejectionReason = "ungrounded_evidence"
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

// Refuse returns the fencing rejection for a reason.
func Refuse(reason RejectionReason) error { return &IdentityError{Reason: reason} }

// CheckRejectionReason accepts only the registered reasons.
func CheckRejectionReason(reason RejectionReason) error {
	switch reason {
	case RejectUnknownEpisode, RejectStaleAttempt, RejectWrongAttempt, RejectTerminalAttempt, RejectEpisodeClosed,
		RejectSchemaInvalid, RejectSnapshotMismatch, RejectEvidenceNotVisible, RejectForgedReference,
		RejectOversized, RejectExpired, RejectIntentTypeNotAllowed, RejectRiskCeilingExceeded,
		RejectCatalogMissing, RejectCatalogForged, RejectIntentTypeNotInCatalog, RejectRiskLabelMismatch,
		RejectParameterSchemaViolated, RejectPresetMismatch, RejectUngroundedEvidence:
		return nil
	default:
		return fmt.Errorf("invalid rejection reason %q", reason)
	}
}

// Rejection is one durable worker rejection. EpisodeID is empty for an
// episode the ledger does not know; the row then carries a null episode.
type Rejection struct {
	ID        string
	EpisodeID string
	AttemptID string
	Fence     int64
	Reason    RejectionReason
	Details   []byte
	At        string
}

// RejectionDetails defaults empty details to an empty JSON object.
func RejectionDetails(details []byte) []byte {
	if len(details) == 0 {
		return []byte(`{}`)
	}
	return details
}

// RejectionID derives the idempotent identity of a rejection from the worker
// identity, the reason, the details and the timestamp text.
func RejectionID(identity Identity, reason RejectionReason, details []byte, at string) string {
	material := fmt.Sprintf("%s|%s|%d|%s|%s|%s", identity.EpisodeID, identity.AttemptID, identity.Fence, reason, string(details), at)
	hash := sha256.Sum256([]byte(material))
	return "rej_" + hex.EncodeToString(hash[:])
}
