package domain

import (
	"encoding/json"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// DecodeDocument decodes and schema-validates a governance contract.
func DecodeDocument(raw []byte, schema contractsv1.SchemaName) (map[string]any, string) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, "schema_invalid"
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, "schema_invalid"
	}
	return document, ""
}

// MatchesDecisionIdentity binds the decision to the accepted episode and Situation.
func MatchesDecisionIdentity(row IntentRecord, document DecisionDocument) bool {
	return document.ID == row.DecisionID &&
		document.EpisodeID == row.EpisodeID &&
		document.SituationID == row.SituationID &&
		document.SituationVersion == row.SituationVersion &&
		row.EpisodeTenant == row.TenantID && row.SituationTenant == row.TenantID &&
		row.DecisionSituation == row.SituationID && row.DecisionVersion == row.SituationVersion &&
		row.EpisodeSituation == row.SituationID && row.EpisodeVersion == row.SituationVersion
}

// MatchesIntentIdentity binds intent fields to their accepted durable projection.
func MatchesIntentIdentity(row IntentRecord, document IntentDocument) bool {
	return document.ID == row.IntentID &&
		document.DecisionID == row.DecisionID &&
		document.TenantID == row.TenantID &&
		document.SituationID == row.SituationID &&
		document.SituationVersion == row.SituationVersion &&
		document.Type == row.IntentType &&
		document.RiskClass == row.RiskClass
}

// EpisodeConcluded permits governance only after the episode has concluded.
func EpisodeConcluded(row IntentRecord) bool {
	return row.EpisodeLifecycle == "concluded" || row.EpisodeLifecycle == "closed"
}

// SourceHealthIncomplete refuses consequential work from incomplete current evidence.
func SourceHealthIncomplete(row IntentRecord) bool {
	consequential := row.RiskClass == "R2" || row.RiskClass == "R3" || row.RiskClass == "R4"
	return consequential && (row.CurrentCompleteness == "provisional" || row.CurrentCompleteness == "uncertain")
}

// ApprovalExpired treats invalid expiry and the deadline itself as expired.
func ApprovalExpired(expiresAt string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	return err != nil || !expires.After(now)
}

// ApprovalDecision chooses lifecycle and intent status for a human decision.
func ApprovalDecision(approved bool) (status, policyStatus string) {
	if approved {
		return "approved", "pending"
	}
	return "denied", "denied"
}
