package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BuildApprovalNotification renders typed governance context into the existing contract.
func BuildApprovalNotification(n ApprovalNotice) ApprovalNotification {
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

func approvalNotificationFields(n ApprovalNotice, summary, hypothesis string) ApprovalNotification {
	return ApprovalNotification{
		TenantID: n.Row.TenantID, ApprovalID: n.ID, IntentID: n.Row.IntentID, DecisionID: n.Row.DecisionID,
		SituationID: n.Row.SituationID, SituationVersion: n.Row.SituationVersion,
		IntentDigest:   "sha256:" + hex.EncodeToString(n.Row.IntentSHA),
		SnapshotDigest: "sha256:" + hex.EncodeToString(n.Context.Snapshot), RiskClass: n.Row.RiskClass,
		ExpiresAt: n.ExpiresAt.UTC().Format(time.RFC3339Nano), Audience: "stream-approval-relay",
		Summary: summary, Delta: objectOrEmpty(n.Context.Delta), Hypothesis: hypothesis, Evidence: n.Intent.Evidence,
		Action: n.Intent.Parameters, DeclineConsequence: "The intent will not be dispatched.",
		SourceAuthority: n.Context.Source,
	}
}

// objectOrEmpty keeps an absent JSON object an empty object, never null.
func objectOrEmpty(object map[string]any) map[string]any {
	if object == nil {
		return map[string]any{}
	}
	return object
}

// CheckBinding requires the notification to name the expected tenant and source
// authority, the values the lifecycle contract stamps on the event.
func (n ApprovalNotification) CheckBinding(tenantID, source string) error {
	if n.TenantID != tenantID || n.SourceAuthority != source {
		return fmt.Errorf("approval notification is not bound to tenant %q and its source authority", tenantID)
	}
	return nil
}
