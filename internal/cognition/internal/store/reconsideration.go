package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) ReconsiderationExists(ctx context.Context, current situations.Version, commandID string) (bool, error) {
	_, found, err := storage.QueryOptional[int](ctx, t.tx, `SELECT 1 FROM reconsiderations WHERE situation_id = ? AND superseded_version = ? AND invalidated_command_id = ?`, current.SituationID, current.PreviousVersion, commandID)
	if err != nil {
		return false, fmt.Errorf("check reconsideration dedupe: %w", err)
	}
	return found, nil
}

func (t *Tx) RecordReconsideration(ctx context.Context, r domain.Reconsideration, correctionDigest []byte, tenantID string, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, `
		INSERT INTO reconsiderations (
			reconsideration_id, tenant_id, situation_id, superseded_version,
			correction_version, correction_snapshot_sha256, invalidated_command_id,
			invalidated_outcome_id, invalidated_outcome_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, tenantID, r.Current.SituationID, r.Current.PreviousVersion,
		r.Current.Version, correctionDigest, r.Command.CommandID, r.Command.OutcomeID, r.Command.OutcomeSHA,
		now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert reconsideration: %w", err)
	}
	return nil
}

func (t *Tx) AnnounceReconsideration(ctx context.Context, r domain.Reconsideration, tenantID string, now time.Time) error {
	if err := notify.AppendLifecycleEvent(ctx, t.tx, reconsiderationEvent(r, tenantID, now)); err != nil {
		return fmt.Errorf("append reconsideration notification: %w", err)
	}
	return nil
}

func reconsiderationEvent(r domain.Reconsideration, tenantID string, now time.Time) notify.LifecycleEvent {
	return notify.LifecycleEvent{
		ID:           "reconsideration.admitted:" + r.ID,
		TenantID:     tenantID,
		Subject:      "situation/" + r.Current.SituationID,
		PartitionKey: r.Current.SituationID,
		Payload:      reconsideration(r),
		At:           now.UTC(),
		Trace:        contractsv1.TraceContext{Traceparent: r.Current.Traceparent, Tracestate: r.Current.Tracestate},
	}
}

func reconsideration(r domain.Reconsideration) notify.ReconsiderationAdmitted {
	return notify.ReconsiderationAdmitted{
		ReconsiderationID: r.ID, SituationID: r.Current.SituationID,
		SupersededVersion: r.Current.PreviousVersion, CorrectionVersion: r.Current.Version,
		InvalidatedCommandID: r.Command.CommandID, InvalidatedOutcomeID: r.Command.OutcomeID,
		TriggerID: r.TriggerID, SchedulerItemID: r.SchedulerItemID,
	}
}

func (t *Tx) InvalidatedCommands(ctx context.Context, current situations.Version) ([]domain.InvalidatedCommand, error) {
	rows, err := t.tx.QueryContext(ctx, selectInvalidatedCommandsSQL, current.SituationID, current.PreviousVersion)
	if err != nil {
		return nil, fmt.Errorf("find invalidated commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return storage.CollectRows(rows, "invalidated commands", scanInvalidatedCommand) //nolint:wrapcheck // CollectRows names the failed step.
}

// selectInvalidatedCommandsSQL finds succeeded commands of the latest
// approved intents at or before the previous version, with their latest
// outcome.
const selectInvalidatedCommandsSQL = `
		SELECT c.command_id, d.decision_id, o.outcome_id, o.ordinal, o.status, o.provider_result_json,
		       o.observed_effect_json, o.reconciliation_status, o.outcome_sha256
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN outcomes o ON o.command_id = c.command_id
		WHERE i.situation_id = ?
		  AND i.situation_version = (
			SELECT MAX(i2.situation_version)
			FROM intents i2
			WHERE i2.situation_id = i.situation_id
			  AND i2.situation_version <= ?
			  AND i2.policy_status = 'approved'
		  )
		  AND d.validation_status = 'accepted' AND c.status = 'succeeded'
		  AND o.ordinal = (SELECT MAX(o2.ordinal) FROM outcomes o2 WHERE o2.command_id = c.command_id)
		ORDER BY c.command_id`

func scanInvalidatedCommand(rows *sql.Rows) (domain.InvalidatedCommand, error) {
	var c domain.InvalidatedCommand
	if err := rows.Scan(&c.CommandID, &c.DecisionID, &c.OutcomeID, &c.OutcomeOrdinal, &c.OutcomeStatus, &c.ProviderJSON, &c.ObservedJSON, &c.ReconciliationStatus, &c.OutcomeSHA); err != nil {
		return domain.InvalidatedCommand{}, fmt.Errorf("scan invalidated command: %w", err)
	}
	return c, nil
}

func (t *Tx) LinkReconsideration(ctx context.Context, r domain.Reconsideration) error {
	if _, err := t.tx.ExecContext(ctx, `UPDATE reconsiderations SET trigger_id = ?, scheduler_item_id = ? WHERE reconsideration_id = ?`, r.TriggerID, r.SchedulerItemID, r.ID); err != nil {
		return fmt.Errorf("link reconsideration admission: %w", err)
	}
	return nil
}

func (t *Tx) SnapshotDigest(ctx context.Context, current situations.Version) ([]byte, error) {
	var persisted []byte
	if err := t.tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", current.SituationID, current.Version).Scan(&persisted); err != nil {
		return nil, fmt.Errorf("load correction snapshot digest: %w", err)
	}
	return persisted, nil
}
