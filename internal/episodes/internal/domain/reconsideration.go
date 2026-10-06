package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// Evaluation is one trigger evaluation as scanned.
type Evaluation struct {
	TriggerID   string
	TriggerName string
	Score       float64
	Threshold   float64
	Lane        string
	DeltaJSON   []byte
}

type ReconsiderationRow struct {
	ReconsiderationID    string
	SituationID          string
	SupersededVersion    int
	CorrectionVersion    int
	InvalidatedCommandID string
	InvalidatedOutcomeID string
	PriorDecisionID      string
	PriorDecisionJSON    []byte
	CommandJSON          []byte
	CommandStatus        string
	IntentID             string
	IntentType           string
	RiskClass            string
	OutcomeID            string
	OutcomeOrdinal       int
	OutcomeStatus        string
	ProviderResultJSON   []byte
	ObservedEffectJSON   []byte
	ReconciliationStatus string
	OutcomeSHA256        []byte
}

func (row ReconsiderationRow) ReconsiderationDocument(ev Evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	priorDecision, err := row.priorDecisionDocument()
	if err != nil {
		return nil, err
	}
	command, err := row.commandDocument()
	if err != nil {
		return nil, err
	}
	outcome, err := row.outcomeDocument()
	if err != nil {
		return nil, err
	}

	correction := row.correctionDocument(ev, delta, snapshot)
	return row.reconsiderationFields(priorDecision, command, outcome, correction), nil
}

func (row ReconsiderationRow) priorDecisionDocument() (map[string]any, error) {
	priorDecision, err := jsonDocument(row.PriorDecisionJSON, "prior decision")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(priorDecision, "decision_id", row.PriorDecisionID); err != nil {
		return nil, err
	}

	return priorDecision, nil
}

// commandDocument is the executed command with its durable identity and status.
func (row ReconsiderationRow) commandDocument() (map[string]any, error) {
	command, err := jsonDocument(row.CommandJSON, "executed command")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "command_id", row.InvalidatedCommandID); err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "intent_id", row.IntentID); err != nil {
		return nil, err
	}
	return row.bindCommandContext(command), nil
}

func (row ReconsiderationRow) bindCommandContext(command map[string]any) map[string]any {
	command["status"] = row.CommandStatus
	command["intent_type"] = row.IntentType
	command["risk_class"] = row.RiskClass
	if _, ok := command["parameters"]; !ok {
		if payload, ok := command["payload"]; ok {
			command["parameters"] = payload
		}
	}
	return command
}

// outcomeDocument is the observed outcome of the invalidated command.
func (row ReconsiderationRow) outcomeDocument() (map[string]any, error) {
	outcome := map[string]any{
		"outcome_id":            row.OutcomeID,
		"command_id":            row.InvalidatedCommandID,
		"ordinal":               row.OutcomeOrdinal,
		"status":                row.OutcomeStatus,
		"outcome_sha256":        canonicaljson.EncodeDigest(row.OutcomeSHA256),
		"reconciliation_status": row.ReconciliationStatus,
	}
	return row.bindOutcomeEvidence(outcome)
}

func (row ReconsiderationRow) bindOutcomeEvidence(outcome map[string]any) (map[string]any, error) {
	provider, err := optionalJSONDocument(row.ProviderResultJSON, "provider result")
	if err != nil {
		return nil, err
	}
	if provider != nil {
		outcome["provider_result"] = provider
	}
	observed, err := optionalJSONDocument(row.ObservedEffectJSON, "observed effect")
	if err != nil {
		return nil, err
	}
	if observed != nil {
		outcome["observed_effect"] = observed
	}
	return outcome, nil
}

func (row ReconsiderationRow) correctionDocument(ev Evaluation, delta, snapshot map[string]any) map[string]any {

	correction := copyDocument(snapshot)
	if nested, ok := delta["correction"].(map[string]any); ok {
		correction = copyDocument(nested)
	}
	correction["reason"] = ev.TriggerName
	correction["invalidates"] = []string{row.InvalidatedCommandID}
	correction["superseded_version"] = row.SupersededVersion
	correction["correction_version"] = row.CorrectionVersion

	return correction
}

func copyDocument(document map[string]any) map[string]any {
	copy := make(map[string]any, len(document))
	for key, value := range document {
		copy[key] = value
	}
	return copy
}

func (row ReconsiderationRow) reconsiderationFields(priorDecision, command, outcome, correction map[string]any) map[string]any {

	return map[string]any{
		"reconsideration_id":     row.ReconsiderationID,
		"situation_id":           row.SituationID,
		"superseded_version":     row.SupersededVersion,
		"correction_version":     row.CorrectionVersion,
		"invalidated_command_id": row.InvalidatedCommandID,
		"invalidated_outcome_id": row.InvalidatedOutcomeID,
		"prior_decision":         priorDecision,
		"commands":               []map[string]any{command},
		"outcomes":               []map[string]any{outcome},
		"correction":             correction,
	}
}

func jsonDocument(raw []byte, name string) (map[string]any, error) {
	var document map[string]any
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if document == nil {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return document, nil
}

func setDocumentIdentity(document map[string]any, key, want string) error {
	if want == "" {
		return fmt.Errorf("%s is required", key)
	}
	if got, ok := document[key]; ok && got != want {
		return fmt.Errorf("%s identity mismatch: got %v, want %s", key, got, want)
	}
	document[key] = want
	return nil
}

func optionalJSONDocument(raw []byte, name string) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return document, nil
}
