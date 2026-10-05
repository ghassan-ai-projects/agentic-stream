package domain

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func DecodeApprovalDecision(raw []byte) (map[string]any, error) {
	var decision map[string]any
	if err := json.Unmarshal(raw, &decision); err != nil {
		return nil, fmt.Errorf("decode approval decision: %w", err)
	}
	return decision, nil
}

func BuildApprovalNotification(row IntentRecord, approvalID string, expiresAt time.Time, intent map[string]any, context ApprovalContext) map[string]any {
	evidence := intentEvidence(intent)
	if parameters, ok := intent["parameters"].(map[string]any); !ok || parameters == nil {
		intent["parameters"] = map[string]any{}
	}
	summary := approvalDecisionText(context.Decision, "summary", "Decision %s requires approval", row.DecisionID)
	hypothesis := approvalDecisionText(context.Decision, "primary_hypothesis", "Decision %s did not record a primary hypothesis", row.DecisionID)
	return approvalNotificationFields(row, approvalID, expiresAt, intent, context, evidence, summary, hypothesis)
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

func approvalDecisionText(decision map[string]any, field, fallback, decisionID string) string {
	value := DocumentString(decision, field)
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf(fallback, decisionID)
	}
	return value
}

func approvalNotificationFields(row IntentRecord, approvalID string, expiresAt time.Time, intent map[string]any, context ApprovalContext, evidence []string, summary, hypothesis string) map[string]any {
	return map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"situation_id": row.SituationID, "situation_version": row.SituationVersion,
		"intent_digest":   "sha256:" + hex.EncodeToString(row.IntentSHA),
		"snapshot_digest": "sha256:" + hex.EncodeToString(context.Snapshot), "risk_class": row.RiskClass,
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano), "audience": "stream-approval-relay",
		"summary": summary, "delta": context.Delta, "hypothesis": hypothesis, "evidence": evidence,
		"action": intent["parameters"], "decline_consequence": "The intent will not be dispatched.",
		"source_authority": context.Source,
	}
}
