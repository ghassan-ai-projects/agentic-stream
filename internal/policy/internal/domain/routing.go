package domain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// RiskRoute chooses the risk rule before considering catalog approval overrides.
func RiskRoute(row IntentRecord) (route, reason string) {
	switch row.RiskClass {
	case "R0", "R1":
		if row.RequiresApproval != 0 {
			return "approval", ""
		}
		return "automatic", ""
	case "R2":
		return "calibration", ""
	case "R3", "R4":
		return "denied", "risk_policy_denied"
	default:
		return "denied", "unknown_risk_class"
	}
}

// FreshnessFailure applies lifecycle, version, health and expiry in that order.
func FreshnessFailure(row IntentRecord, now time.Time) (status, reason string, expires time.Time) {
	if !EpisodeConcluded(row) {
		return "denied", "episode_not_concluded", time.Time{}
	}
	if row.CurrentSituation != row.SituationVersion {
		return "stale", "situation_version_stale", time.Time{}
	}
	if SourceHealthIncomplete(row) {
		return "denied", "source_health_incomplete", time.Time{}
	}
	expires, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expires.After(now) {
		return "expired", "intent_expired", time.Time{}
	}
	return "", "", expires
}

// CompensationFailure binds compensation targets to the same tenant.
func CompensationFailure(tenant, commandTenant string, found bool) string {
	if !found {
		return "compensation_target_missing"
	}
	if tenant != commandTenant {
		return "compensation_tenant_mismatch"
	}
	return ""
}

// ApprovalDisposition preserves already-resolved, stale, then expired precedence.
func ApprovalDisposition(row IntentRecord, a ApprovalRecord, r ApprovalResolution) string {
	if a.Status != "pending" {
		return "resolved"
	}
	if r.Approved && row.CurrentSituation != row.SituationVersion {
		return "stale"
	}
	if ApprovalExpired(a.ExpiresAt, r.Now) {
		return "expired"
	}
	return "authorize"
}

// DistinctPrincipals refuses empty or self-relayed human approvals.
func DistinctPrincipals(r ApprovalResolution) error {
	if r.Approver == "" || r.Relay == "" || r.Approver == r.Relay {
		return fmt.Errorf("relay and approver must be distinct registered principals")
	}
	return nil
}

// ApprovalNonce preserves the request's stable single-use identity.
func ApprovalNonce(approvalID, intentID string) string {
	digest := sha256.Sum256([]byte(approvalID + "|" + intentID))
	return hex.EncodeToString(digest[:])
}

// VerifyAssertion verifies the immutable signed approval document.
func VerifyAssertion(publicKey, assertion, signature []byte) ([]byte, error) {
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(publicKey), assertion, signature) {
		return nil, fmt.Errorf("approval assertion signature is invalid")
	}
	digest := sha256.Sum256(assertion)
	return digest[:], nil
}

// CompleteDigest refuses incomplete approval evidence.
func CompleteDigest(digest []byte, kind string) error {
	if len(digest) != sha256.Size {
		return fmt.Errorf("%s digest is incomplete", kind)
	}
	return nil
}

// ActiveRelay requires exactly one active registered tenant relay.
func ActiveRelay(active int, readFailed bool) error {
	if readFailed || active != 1 {
		return fmt.Errorf("relay principal is not active")
	}
	return nil
}

// ValidApproverKey requires an active tenant principal with an Ed25519 key.
func ValidApproverKey(key []byte, readFailed bool) error {
	if readFailed || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("approver principal has no valid verification key")
	}
	return nil
}

// AuthorizedApprover requires a role bound to this tenant, entity and risk.
func AuthorizedApprover(active int, readFailed bool) error {
	if readFailed || active == 0 {
		return fmt.Errorf("approver principal lacks authority")
	}
	return nil
}
