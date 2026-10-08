package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// AdmitPendingSchedulerItem marks a pending item admitted and returns the rows it changed.
func (t *Tx) AdmitPendingSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'admitted', updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.Format(time.RFC3339Nano), schedulerItemID)
	if err != nil {
		return 0, fmt.Errorf("mark scheduler item admitted: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return count, nil
}

// CoalesceTriggerItems marks the trigger's open queue items replaced by newer work.
func (t *Tx) CoalesceTriggerItems(ctx context.Context, situationID, triggerName, now string) error {
	if _, err := t.q.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')`,
		now, situationID, situationID, triggerName,
	); err != nil {
		return fmt.Errorf("supersede scheduler items: %w", err)
	}
	return nil
}

// CoalescePendingSchedulerItem removes one pending item from the queue and
// returns the rows it changed; operation labels errors.
func (t *Tx) CoalescePendingSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time, operation string) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.UTC().Format(time.RFC3339Nano), schedulerItemID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows affected: %w", operation, err)
	}
	return count, nil
}

// NextPendingSchedulerItem returns the tenant's next pending item that is due
// at now, in queue order: not_before, then creation, then identity.
func (t *Tx) NextPendingSchedulerItem(ctx context.Context, tenantID string, now time.Time) (string, bool, error) {
	itemID, found, err := storage.QueryOptional[string](ctx, t.q, `
		SELECT scheduler_item_id FROM scheduler_items
		WHERE tenant_id = ? AND status = 'pending' AND (not_before IS NULL OR not_before <= ?)
		ORDER BY not_before, created_at, scheduler_item_id LIMIT 1`,
		tenantID, now.Format(time.RFC3339Nano))
	if err != nil {
		return "", false, fmt.Errorf("find pending scheduler item: %w", err)
	}
	return itemID, found, nil
}
