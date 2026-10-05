package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BuildApprovalNotification renders typed governance context into the existing contract.
func BuildApprovalNotification(n ApprovalNotice) map[string]any {
	if n.Intent.Parameters == nil {
		n.Intent.Parameters = map[string]any{}
		n.Intent.Document["parameters"] = n.Intent.Parameters
	}
	summary := approvalDecisionText(n.Context.Decision.Summary, "Decision %s requires approval", n.Row.DecisionID)
	hypothesis := approvalDecisionText(n.Context.Decision.Hypothesis, "Decision %s did not record a primary hypothesis", n.Row.DecisionID)
	return approvalNotificationFields(n, summary, hypothesis)
}

func intentEvidence(intent map[string]any) []string {
	values, _ := intent["evidence_ids"].([]any)
	evidence := make([]string, 0, len(values))
	for _, value := range values {
		if id, ok := value.(string); ok && id != "" {
			evidence = append(evidence, id)
		}
	}
	return evidence
}

func approvalDecisionText(value, fallback, decisionID string) string {
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf(fallback, decisionID)
	}
	return value
}

func approvalNotificationFields(n ApprovalNotice, summary, hypothesis string) map[string]any {
	return map[string]any{
		"tenant_id": n.Row.TenantID, "approval_id": n.ID, "intent_id": n.Row.IntentID, "decision_id": n.Row.DecisionID,
		"situation_id": n.Row.SituationID, "situation_version": n.Row.SituationVersion,
		"intent_digest":   "sha256:" + hex.EncodeToString(n.Row.IntentSHA),
		"snapshot_digest": "sha256:" + hex.EncodeToString(n.Context.Snapshot), "risk_class": n.Row.RiskClass,
		"expires_at": n.ExpiresAt.UTC().Format(time.RFC3339Nano), "audience": "stream-approval-relay",
		"summary": summary, "delta": n.Context.Delta, "hypothesis": hypothesis, "evidence": n.Intent.Evidence,
		"action": n.Intent.Parameters, "decline_consequence": "The intent will not be dispatched.",
		"source_authority": n.Context.Source,
	}
}
