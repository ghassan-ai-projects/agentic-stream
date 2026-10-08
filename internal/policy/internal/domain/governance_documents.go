package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// DecisionDocument is a closed governance view retaining the original digest input.
type DecisionDocument struct {
	Document                   map[string]any
	ID, EpisodeID, SituationID string
	SituationVersion           int
	Summary, Hypothesis        string
}

// IntentDocument is a closed governance view; Parameters remain effector-owned data.
type IntentDocument struct {
	Document                                                            map[string]any
	ID, DecisionID, TenantID, SituationID, Type, RiskClass, Compensates string
	SituationVersion                                                    int
	Parameters                                                          map[string]any
	Evidence                                                            []string
}

// GovernanceDocuments are the validated decision and intent for one evaluation.
type GovernanceDocuments struct {
	Decision DecisionDocument
	Intent   IntentDocument
}

// ParseGovernanceDocuments preserves decision-before-intent failure precedence.
func ParseGovernanceDocuments(row IntentRecord) (GovernanceDocuments, string) {
	decision, reason := ParseDecision(row)
	if reason != "" {
		return GovernanceDocuments{}, reason
	}
	intent, reason := ParseIntent(row)
	return GovernanceDocuments{Decision: decision, Intent: intent}, reason
}

// ParseDecision validates schema, identity and digest on one decoded document.
func ParseDecision(row IntentRecord) (DecisionDocument, string) {
	if row.ValidationStatus != "accepted" {
		return DecisionDocument{}, "decision_not_accepted"
	}
	document, reason := DecodeDocument(row.DecisionJSON, contractsv1.SchemaDecision)
	if reason != "" {
		return DecisionDocument{}, reason
	}
	decision := projectDecision(document)
	if !MatchesDecisionIdentity(row, decision) {
		return DecisionDocument{}, "identity_mismatch"
	}
	if !DocumentDigestMatches(document, row.DecisionSHA, canonicaljson.DomainDecision) {
		return DecisionDocument{}, "decision_digest_mismatch"
	}
	return decision, ""
}
func projectDecision(document map[string]any) DecisionDocument {
	return DecisionDocument{Document: document, ID: contractsv1.DocumentString(document, "decision_id"), EpisodeID: contractsv1.DocumentString(document, "episode_id"), SituationID: contractsv1.DocumentString(document, "situation_id"), SituationVersion: contractsv1.DocumentInt(document, "situation_version"), Summary: contractsv1.DocumentString(document, "summary"), Hypothesis: contractsv1.DocumentString(document, "primary_hypothesis")}
}

// ParseIntent validates schema and digest before compensation and identity checks.
func ParseIntent(row IntentRecord) (IntentDocument, string) {
	document, reason := DecodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
	if reason != "" {
		return IntentDocument{}, reason
	}
	if !DocumentDigestMatches(document, row.IntentSHA, canonicaljson.DomainIntent) {
		return IntentDocument{}, "intent_digest_mismatch"
	}
	return ProjectIntent(document), ""
}

// ProjectIntent selects governance fields while retaining the original document.
func ProjectIntent(document map[string]any) IntentDocument {
	parameters, _ := document["parameters"].(map[string]any)
	return IntentDocument{Document: document, ID: contractsv1.DocumentString(document, "intent_id"), DecisionID: contractsv1.DocumentString(document, "decision_id"), TenantID: contractsv1.DocumentString(document, "tenant_id"), SituationID: contractsv1.DocumentString(document, "situation_id"), SituationVersion: contractsv1.DocumentInt(document, "situation_version"), Type: contractsv1.DocumentString(document, "type"), RiskClass: contractsv1.DocumentString(document, "risk_class"), Compensates: contractsv1.DocumentString(document, "compensates"), Parameters: parameters, Evidence: intentEvidence(document)}
}
