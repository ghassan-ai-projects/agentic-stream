package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ApprovalRequestJSON seals the original intent and its approval presentation.
func ApprovalRequestJSON(row IntentRecord, intent IntentDocument, request ApprovalRequest) ([]byte, error) {
	raw, err := canonicaljson.Marshal(map[string]any{
		"approval_id": request.ID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"tenant_id": row.TenantID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "risk_class": row.RiskClass,
		"intent": intent.Document, "notification": request.Data, "nonce": request.Nonce,
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize approval: %w", err)
	}
	return raw, nil
}
