package cognition

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"time"
)

func (s *Scheduler) supersedePending(ctx context.Context, tx *sql.Tx, situationID, triggerName string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	var tenantID string
	var replacementVersion int
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, current_version FROM situations WHERE situation_id = ?", situationID).Scan(&tenantID, &replacementVersion); err != nil {
		return fmt.Errorf("load supersession situation: %w", err)
	}
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT traceparent, tracestate FROM situation_versions WHERE situation_id = ? AND version = ?", situationID, replacementVersion).Scan(&traceparent, &tracestate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load supersession trace: %w", err)
	}
	type supersededItem struct {
		id      string
		version int
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT scheduler_item_id, situation_version
		FROM scheduler_items
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')
		ORDER BY scheduler_item_id`, situationID, situationID, triggerName)
	if err != nil {
		return fmt.Errorf("find superseded scheduler items: %w", err)
	}
	items := make([]supersededItem, 0)
	for rows.Next() {
		var item supersededItem
		if err := rows.Scan(&item.id, &item.version); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan superseded scheduler item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate superseded scheduler items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close superseded scheduler items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')`,
		now, situationID, situationID, triggerName,
	); err != nil {
		return fmt.Errorf("supersede scheduler items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE scheduler_item_id IN (
			SELECT scheduler_item_id FROM scheduler_items
			WHERE situation_id = ? AND status = 'coalesced'
		) AND lifecycle_status IN ('admitted', 'running')`,
		now, situationID,
	); err != nil {
		return fmt.Errorf("supersede episodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episode_attempts SET status = 'cancelling'
		WHERE episode_id IN (
			SELECT episode_id FROM episodes
			WHERE scheduler_item_id IN (
				SELECT scheduler_item_id FROM scheduler_items
				WHERE situation_id = ? AND status = 'coalesced'
			) AND lifecycle_status = 'superseded'
		) AND status IN ('dispatched', 'running')`,
		situationID,
	); err != nil {
		return fmt.Errorf("cancel superseded attempts: %w", err)
	}
	for _, item := range items {
		if item.version >= replacementVersion {
			continue
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx,
			"situation.superseded:"+situationID+":"+fmt.Sprint(item.version)+":"+fmt.Sprint(replacementVersion)+":"+item.id,
			tenantID, notify.TypeSituationSuperseded, "situation/"+situationID, situationID,
			map[string]any{
				"tenant_id": tenantID, "situation_id": situationID,
				"superseded_version": item.version, "replacement_version": replacementVersion,
				"reason": "newer_situation_version_admitted", "source_authority": notify.SourceForTenant(tenantID),
			}, s.clk.Now().UTC(), contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}); err != nil {
			return fmt.Errorf("append situation superseded notification: %w", err)
		}
	}
	if err := s.withdrawSupersededApprovals(ctx, tx, situationID, tenantID, replacementVersion, now); err != nil {
		return err
	}
	return nil
}

func (s *Scheduler) withdrawSupersededApprovals(ctx context.Context, tx *sql.Tx, situationID, tenantID string, replacementVersion int, now string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.approval_id, i.intent_id, i.situation_id, i.situation_version,
		       d.traceparent, d.tracestate
		FROM approvals a
		JOIN intents i ON i.intent_id = a.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE i.situation_id = ? AND i.situation_version < ? AND a.status = 'pending'
		ORDER BY a.approval_id`, situationID, replacementVersion)
	if err != nil {
		return fmt.Errorf("find superseded approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var approvalID, intentID, linkedSituationID string
		var situationVersion int
		var traceparent, tracestate sql.NullString
		if err := rows.Scan(&approvalID, &intentID, &linkedSituationID, &situationVersion, &traceparent, &tracestate); err != nil {
			return fmt.Errorf("scan superseded approval: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE approvals SET status = 'denied', decided_at = ?, withdrawn_at = ?, withdrawal_reason = ?, reason = ?
			WHERE approval_id = ? AND status = 'pending'`, now, now, "situation_version_conflict", "approval_withdrawn", approvalID); err != nil {
			return fmt.Errorf("withdraw superseded approval %s: %w", approvalID, err)
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.withdrawn:"+approvalID, tenantID, notify.TypeApprovalWithdrawn, "approval/"+approvalID, linkedSituationID, map[string]any{
			"tenant_id": tenantID, "approval_id": approvalID, "intent_id": intentID, "situation_id": linkedSituationID,
			"situation_version": situationVersion, "reason": "situation_version_conflict", "source_authority": notify.SourceForTenant(tenantID),
		}, s.clk.Now().UTC(), contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}); err != nil {
			return fmt.Errorf("append superseded approval notification: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate superseded approvals: %w", err)
	}
	return nil
}
