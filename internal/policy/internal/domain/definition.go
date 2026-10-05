package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func canonicalApprovalAssertion(assertion ApprovalAssertion) ([]byte, error) {
	result, err := canonicaljson.Marshal(map[string]any{
		"approval_id": assertion.ApprovalID, "intent_id": assertion.IntentID, "decision_id": assertion.DecisionID,
		"tenant_id": assertion.TenantID, "situation_id": assertion.SituationID,
		"situation_version": assertion.SituationVersion, "risk_class": assertion.RiskClass,
		"intent_digest": assertion.IntentDigest, "decision_digest": assertion.DecisionDigest,
		"expires_at": assertion.ExpiresAt, "nonce": assertion.Nonce,
		"approver_id": assertion.ApproverID, "relay_id": assertion.RelayID,
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize approval assertion: %w", err)
	}
	return result, nil
}

func ApprovalAssertionSigningBytes(assertion ApprovalAssertion) ([]byte, error) {
	canonical, err := canonicalApprovalAssertion(assertion)
	if err != nil {
		return nil, err
	}
	return append([]byte(canonicaljson.DomainApproval), canonical...), nil
}

func DigestForVersion(policyVersion string) (string, error) {
	if policyVersion == "" {
		return "", fmt.Errorf("policy version is required")
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainPolicy, CanonicalDocumentForVersion(policyVersion))
	if err != nil {
		return "", fmt.Errorf("digest policy document: %w", err)
	}
	return digest, nil
}

func CanonicalDocumentForVersion(policyVersion string) map[string]any {
	return map[string]any{
		"policy_version": policyVersion,
		"risk_policy": map[string]any{
			"R0": "automatic", "R1": "automatic", "R2": "approval", "R3": "denied", "R4": "denied",
		},
		"incomplete_source_health": map[string]any{"R2": "denied", "R3": "denied", "R4": "denied"},
		"target_resolution":        "closed_catalog_binding",
	}
}
