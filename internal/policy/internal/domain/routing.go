package domain

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func RiskRoute(row IntentRecord) (route, reason string) {
	risk := contractsv1.RiskClass(row.RiskClass)
	routed := contractsv1.RouteFor(risk, row.RequiresApproval != 0)
	return string(routed), denialReason(risk, routed)
}

func denialReason(risk contractsv1.RiskClass, route contractsv1.Route) string {
	switch {
	case !risk.Valid():
		return "unknown_risk_class"
	case route == contractsv1.RouteDenied:
		return "risk_policy_denied"
	default:
		return ""
	}
}

func FreshnessFailure(row IntentRecord, now time.Time) (status, reason string, expires time.Time) {
	if !row.EpisodeProducedDecision {
		return "denied", "episode_not_concluded", time.Time{}
	}
	if MateriallySuperseded(row) {
		return "stale", "situation_version_stale", time.Time{}
	}
	if SourceHealthIncomplete(row) {
		return "denied", "source_health_incomplete", time.Time{}
	}
	if status, reason := expiryFailure(row, now); reason != "" {
		return status, reason, time.Time{}
	}
	return "", "", row.ExpiresAt
}

func expiryFailure(row IntentRecord, now time.Time) (status, reason string) {
	if row.ExpiryUnreadable {
		return "denied", "intent_expiry_unreadable"
	}
	if !row.ExpiresAt.After(now) {
		return "expired", "intent_expired"
	}
	return "", ""
}

func MateriallySuperseded(row IntentRecord) bool {
	return row.LastMaterialVersion > row.SituationVersion
}

func CompensationFailure(tenant, commandTenant string, found bool) string {
	if !found {
		return "compensation_target_missing"
	}
	if tenant != commandTenant {
		return "compensation_tenant_mismatch"
	}
	return ""
}

func ApprovalDisposition(row IntentRecord, a ApprovalRecord, r ApprovalResolution) string {
	if a.Status != "pending" {
		return "resolved"
	}
	if r.Approved && MateriallySuperseded(row) {
		return "stale"
	}
	if !a.ExpiresAt.After(r.Now) {
		return "expired"
	}
	return "authorize"
}

func DistinctPrincipals(r ApprovalResolution) error {
	if r.Approver == "" || r.Relay == "" || r.Approver == r.Relay {
		return fmt.Errorf("relay and approver must be distinct registered principals")
	}
	return nil
}

func ApprovalNonce(approvalID, intentID string) string {
	return hex.EncodeToString(canonicaljson.Sum([]byte(approvalID + "|" + intentID)))
}

func VerifyAssertion(publicKey, assertion, signature []byte) ([]byte, error) {
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(publicKey), assertion, signature) {
		return nil, fmt.Errorf("approval assertion signature is invalid")
	}
	return canonicaljson.Sum(assertion), nil
}

func CompleteDigest(digest []byte, kind string) error {
	if !canonicaljson.HasSumLength(digest) {
		return fmt.Errorf("%s digest is incomplete", kind)
	}
	return nil
}

func ActiveRelay(active int, readFailed bool) error {
	if readFailed || active != 1 {
		return fmt.Errorf("relay principal is not active")
	}
	return nil
}

func ValidApproverKey(key []byte, readFailed bool) error {
	if readFailed || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("approver principal has no valid verification key")
	}
	return nil
}

func AuthorizedApprover(active int, readFailed bool) error {
	if readFailed || active == 0 {
		return fmt.Errorf("approver principal lacks authority")
	}
	return nil
}
