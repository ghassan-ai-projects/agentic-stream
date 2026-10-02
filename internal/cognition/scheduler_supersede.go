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

// supersedePending coalesces the trigger's pending and admitted scheduler
// items for the Situation, supersedes their episodes and cancels their
// attempts, then announces each superseded older version and withdraws its
// pending approvals.
func (s *Scheduler) supersedePending(ctx context.Context, tx *sql.Tx, situationID, triggerName string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	replacement, err := loadReplacement(ctx, tx, situationID)
	if err != nil {
		return err
	}
	items, err := supersededItems(ctx, tx, situationID, triggerName)
	if err != nil {
		return err
	}
	if err := coalesceTriggerWork(ctx, tx, situationID, triggerName, now); err != nil {
		return err
	}
	if err := s.announceSuperseded(ctx, tx, replacement, items); err != nil {
		return err
	}
	return s.withdrawSupersededApprovals(ctx, tx, situationID, replacement.tenantID, replacement.version, now)
}

// replacementVersion is the Situation's current version, which supersedes
// older pending work, with its trace context.
type replacementVersion struct {
	situationID, tenantID   string
	version                 int
	traceparent, tracestate sql.NullString
}

func loadReplacement(ctx context.Context, tx *sql.Tx, situationID string) (replacementVersion, error) {
	r := replacementVersion{situationID: situationID}
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, current_version FROM situations WHERE situation_id = ?", situationID).Scan(&r.tenantID, &r.version); err != nil {
		return replacementVersion{}, fmt.Errorf("load supersession situation: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT traceparent, tracestate FROM situation_versions WHERE situation_id = ? AND version = ?", situationID, r.version).Scan(&r.traceparent, &r.tracestate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return replacementVersion{}, fmt.Errorf("load supersession trace: %w", err)
	}
	return r, nil
}

type supersededItem struct {
	id      string
	version int
}

// supersededItems lists the trigger's pending and admitted scheduler items
// for the Situation.
func supersededItems(ctx context.Context, tx *sql.Tx, situationID, triggerName string) ([]supersededItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT scheduler_item_id, situation_version
		FROM scheduler_items
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')
		ORDER BY scheduler_item_id`, situationID, situationID, triggerName)
	if err != nil {
		return nil, fmt.Errorf("find superseded scheduler items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]supersededItem, 0)
	for rows.Next() {
		var item supersededItem
		if err := rows.Scan(&item.id, &item.version); err != nil {
			return nil, fmt.Errorf("scan superseded scheduler item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate superseded scheduler items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close superseded scheduler items: %w", err)
	}
	return items, nil
}

// coalesceTriggerWork coalesces the trigger's open scheduler items,
// supersedes their live episodes, and cancels those episodes' attempts.
func coalesceTriggerWork(ctx context.Context, tx *sql.Tx, situationID, triggerName, now string) error {
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
	return nil
}

// announceSuperseded appends a situation.superseded notification for every
// item bound to a version older than the replacement.
func (s *Scheduler) announceSuperseded(ctx context.Context, tx *sql.Tx, replacement replacementVersion, items []supersededItem) error {
	situationID, tenantID := replacement.situationID, replacement.tenantID
	trace := contractsv1.TraceContext{Traceparent: replacement.traceparent.String, Tracestate: replacement.tracestate.String}
	for _, item := range items {
		if item.version >= replacement.version {
			continue
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx,
			"situation.superseded:"+situationID+":"+fmt.Sprint(item.version)+":"+fmt.Sprint(replacement.version)+":"+item.id,
			tenantID, notify.TypeSituationSuperseded, "situation/"+situationID, situationID,
			map[string]any{
				"tenant_id": tenantID, "situation_id": situationID,
				"superseded_version": item.version, "replacement_version": replacement.version,
				"reason": "newer_situation_version_admitted", "source_authority": notify.SourceForTenant(tenantID),
			}, s.clk.Now().UTC(), trace); err != nil {
			return fmt.Errorf("append situation superseded notification: %w", err)
		}
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
