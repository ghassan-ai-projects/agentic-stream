package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type priorDocuments struct {
	decision, command, outcome map[string]any
}

func (c InvalidatedCommand) priorDocuments() (priorDocuments, error) {
	decision, err := c.priorDecision()
	if err != nil {
		return priorDocuments{}, err
	}
	command, err := c.priorCommand()
	if err != nil {
		return priorDocuments{}, err
	}
	return priorDocuments{decision: decision, command: command, outcome: c.PriorOutcome()}, nil
}

func (c InvalidatedCommand) priorDecision() (map[string]any, error) {
	decision, err := jsonDocument(c.DecisionJSON, "prior decision")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(decision, "decision_id", c.DecisionID); err != nil {
		return nil, err
	}
	return decision, nil
}

func (c InvalidatedCommand) priorCommand() (map[string]any, error) {
	command, err := jsonDocument(c.CommandJSON, "executed command")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "command_id", c.CommandID); err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "intent_id", c.IntentID); err != nil {
		return nil, err
	}
	return c.bindCommandContext(command), nil
}

func (c InvalidatedCommand) bindCommandContext(command map[string]any) map[string]any {
	command["status"] = c.CommandStatus
	command["intent_type"] = c.IntentType
	command["risk_class"] = c.RiskClass
	if _, ok := command["parameters"]; !ok {
		if payload, ok := command["payload"]; ok {
			command["parameters"] = payload
		}
	}
	return command
}

func (c InvalidatedCommand) PriorOutcome() map[string]any {
	outcome := map[string]any{
		"status": c.OutcomeStatus, "reconciliation_status": c.ReconciliationStatus,
		"outcome_id": c.OutcomeID, "command_id": c.CommandID, "ordinal": c.OutcomeOrdinal,
		"outcome_sha256": canonicaljson.EncodeDigest(c.OutcomeSHA),
	}
	if len(c.ProviderJSON) > 0 {
		outcome["provider_result"] = json.RawMessage(c.ProviderJSON)
	}
	if len(c.ObservedJSON) > 0 {
		outcome["observed_effect"] = json.RawMessage(c.ObservedJSON)
	}
	return outcome
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
