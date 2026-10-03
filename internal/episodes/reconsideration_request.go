package episodes

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type reconsiderationRow struct {
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
	ReconciliationStatus sql.NullString
	OutcomeSHA256        []byte
}

const queryReconsiderationSQL = `
		SELECT r.reconsideration_id, r.situation_id, r.superseded_version, r.correction_version,
		       r.invalidated_command_id, r.invalidated_outcome_id,
		       d.decision_id, d.raw_json, c.command_json, c.status, i.intent_id, i.intent_type, i.risk_class,
		       o.outcome_id, o.ordinal, o.status, o.provider_result_json, o.observed_effect_json,
		       o.reconciliation_status, o.outcome_sha256
		FROM reconsiderations r
		JOIN commands c ON c.command_id = r.invalidated_command_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN outcomes o ON o.command_id = c.command_id AND o.outcome_id = r.invalidated_outcome_id
		WHERE r.tenant_id = ? AND r.situation_id = ? AND r.correction_version = ?
		  AND (
				r.scheduler_item_id = ? OR
				r.trigger_id = ? OR
				(r.superseded_version = ? AND r.invalidated_command_id = ?)
		  )
		ORDER BY CASE
			WHEN r.scheduler_item_id = ? THEN 0
			WHEN r.trigger_id = ? THEN 1
			ELSE 2
		END
		LIMIT 1`

func loadReconsideration(ctx context.Context, tx *sql.Tx, item schedulerItem, ev evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	row, err := queryReconsideration(ctx, tx, item, delta)
	if err != nil {
		return nil, err
	}
	return row.reconsiderationDocument(ev, delta, snapshot)
}

// queryReconsideration loads the invalidated command, its decision, and its
// outcome for a reconsider item, preferring the exact scheduler item, then
// the trigger, then the superseded version and command.
func queryReconsideration(ctx context.Context, tx *sql.Tx, item schedulerItem, delta map[string]any) (reconsiderationRow, error) {
	supersededVersion := snapshotInt(delta, "superseded_version")
	invalidatedCommandID := snapshotString(delta, "invalidated_command_id")
	var row reconsiderationRow
	query := tx.QueryRowContext(ctx, queryReconsiderationSQL, item.TenantID, item.SituationID, item.SituationVersion,
		item.SchedulerItemID, item.TriggerID, supersededVersion, invalidatedCommandID, item.SchedulerItemID, item.TriggerID)
	if err := scanReconsideration(query, &row); err != nil {
		return reconsiderationRow{}, err
	}

	return row, nil
}

func scanReconsideration(query *sql.Row, row *reconsiderationRow) error {
	if err := query.Scan(
		&row.ReconsiderationID, &row.SituationID, &row.SupersededVersion, &row.CorrectionVersion,
		&row.InvalidatedCommandID, &row.InvalidatedOutcomeID,
		&row.PriorDecisionID, &row.PriorDecisionJSON, &row.CommandJSON, &row.CommandStatus, &row.IntentID, &row.IntentType, &row.RiskClass,
		&row.OutcomeID, &row.OutcomeOrdinal, &row.OutcomeStatus, &row.ProviderResultJSON, &row.ObservedEffectJSON,
		&row.ReconciliationStatus, &row.OutcomeSHA256,
	); err != nil {
		return fmt.Errorf("query reconsideration evidence: %w", err)
	}

	return nil
}

func (row reconsiderationRow) reconsiderationDocument(ev evaluation, delta, snapshot map[string]any) (map[string]any, error) {
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

func (row reconsiderationRow) priorDecisionDocument() (map[string]any, error) {
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
func (row reconsiderationRow) commandDocument() (map[string]any, error) {
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

func (row reconsiderationRow) bindCommandContext(command map[string]any) map[string]any {
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
func (row reconsiderationRow) outcomeDocument() (map[string]any, error) {
	outcome := map[string]any{
		"outcome_id":            row.OutcomeID,
		"command_id":            row.InvalidatedCommandID,
		"ordinal":               row.OutcomeOrdinal,
		"status":                row.OutcomeStatus,
		"outcome_sha256":        "sha256:" + hex.EncodeToString(row.OutcomeSHA256),
		"reconciliation_status": row.ReconciliationStatus.String,
	}
	return row.bindOutcomeEvidence(outcome)
}

func (row reconsiderationRow) bindOutcomeEvidence(outcome map[string]any) (map[string]any, error) {
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

func (row reconsiderationRow) correctionDocument(ev evaluation, delta, snapshot map[string]any) map[string]any {

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

func (row reconsiderationRow) reconsiderationFields(priorDecision, command, outcome, correction map[string]any) map[string]any {

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
