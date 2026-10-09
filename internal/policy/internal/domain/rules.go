package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// DecodeDocument decodes and schema-validates a governance contract.
func DecodeDocument(raw []byte, schema contractsv1.SchemaName) (map[string]any, string) {
	document, err := contractsv1.DecodeDocument(raw, schema)
	if err != nil {
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

// SourceHealthIncomplete refuses consequential work from incomplete current evidence.
func SourceHealthIncomplete(row IntentRecord) bool {
	consequential := contractsv1.RiskClass(row.RiskClass).Consequential()
	return consequential && (row.CurrentCompleteness == "provisional" || row.CurrentCompleteness == "uncertain")
}

// ApprovalDecision chooses lifecycle and intent status for a human decision.
func ApprovalDecision(approved bool) (status, policyStatus string) {
	if approved {
		return "approved", "pending"
	}
	return "denied", "denied"
}
