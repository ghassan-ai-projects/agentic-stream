package domain

import (
	"encoding/json"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func PendingDecisionFailure(row IntentRecord) string {
	if row.ValidationStatus != "accepted" {
		return "decision_not_accepted"
	}
	decision, reason := DecodeDocument(row.DecisionJSON, contractsv1.SchemaDecision)
	if reason != "" {
		return reason
	}
	if !MatchesDecisionIdentity(row, decision) {
		return "identity_mismatch"
	}
	if !CanonicalDocumentMatches(row.DecisionJSON, row.DecisionSHA, canonicaljson.DomainDecision) {
		return "decision_digest_mismatch"
	}
	return ""
}

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

func MatchesDecisionIdentity(row IntentRecord, document map[string]any) bool {
	return DocumentString(document, "decision_id") == row.DecisionID &&
		DocumentString(document, "episode_id") == row.EpisodeID &&
		DocumentString(document, "situation_id") == row.SituationID &&
		DocumentInt(document, "situation_version") == row.SituationVersion &&
		row.EpisodeTenant == row.TenantID && row.SituationTenant == row.TenantID &&
		row.DecisionSituation == row.SituationID && row.DecisionVersion == row.SituationVersion &&
		row.EpisodeSituation == row.SituationID && row.EpisodeVersion == row.SituationVersion
}

func MatchesIntentIdentity(row IntentRecord, document map[string]any) bool {
	return DocumentString(document, "intent_id") == row.IntentID &&
		DocumentString(document, "decision_id") == row.DecisionID &&
		DocumentString(document, "tenant_id") == row.TenantID &&
		DocumentString(document, "situation_id") == row.SituationID &&
		DocumentInt(document, "situation_version") == row.SituationVersion &&
		DocumentString(document, "type") == row.IntentType &&
		DocumentString(document, "risk_class") == row.RiskClass
}

func EpisodeConcluded(row IntentRecord) bool {
	return row.EpisodeLifecycle == "concluded" || row.EpisodeLifecycle == "closed"
}

func SourceHealthIncomplete(row IntentRecord) bool {
	consequential := row.RiskClass == "R2" || row.RiskClass == "R3" || row.RiskClass == "R4"
	return consequential && (row.CurrentCompleteness == "provisional" || row.CurrentCompleteness == "uncertain")
}

func ApprovalExpired(expiresAt string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	return err != nil || !expires.After(now)
}

func ApprovalDecision(approved bool) (status, policyStatus string) {
	if approved {
		return "approved", "pending"
	}
	return "denied", "denied"
}
