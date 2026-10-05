package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ReconsiderationRow is the reconsideration evidence join as scanned: the
// reconsideration, its prior decision, the invalidated command's intent and
// the invalidated outcome.
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

// LoadReconsideration loads the invalidated command, its decision, and its
// outcome for a reconsider item, preferring the exact scheduler item, then
// the trigger, then the superseded version and command.
func LoadReconsideration(ctx context.Context, tx *sql.Tx, item SchedulerItem, supersededVersion int, invalidatedCommandID string) (ReconsiderationRow, error) {
	var row ReconsiderationRow
	query := tx.QueryRowContext(ctx, queryReconsiderationSQL, item.TenantID, item.SituationID, item.SituationVersion,
		item.SchedulerItemID, item.TriggerID, supersededVersion, invalidatedCommandID, item.SchedulerItemID, item.TriggerID)
	if err := query.Scan(
		&row.ReconsiderationID, &row.SituationID, &row.SupersededVersion, &row.CorrectionVersion,
		&row.InvalidatedCommandID, &row.InvalidatedOutcomeID,
		&row.PriorDecisionID, &row.PriorDecisionJSON, &row.CommandJSON, &row.CommandStatus, &row.IntentID, &row.IntentType, &row.RiskClass,
		&row.OutcomeID, &row.OutcomeOrdinal, &row.OutcomeStatus, &row.ProviderResultJSON, &row.ObservedEffectJSON,
		&row.ReconciliationStatus, &row.OutcomeSHA256,
	); err != nil {
		return ReconsiderationRow{}, fmt.Errorf("query reconsideration evidence: %w", err)
	}
	return row, nil
}
