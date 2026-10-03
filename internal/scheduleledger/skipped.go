package scheduleledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CoalesceCostRejected prevents a cost-rejected opportunity from blocking the queue.
func CoalesceCostRejected(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.UTC().Format(time.RFC3339Nano), schedulerItemID)
	if err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cost-rejected scheduler item rows affected: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}

// CoalesceSkipped removes an unavailable opportunity from the pending queue.
func CoalesceSkipped(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.UTC().Format(time.RFC3339Nano), schedulerItemID,
	)
	if err != nil {
		return fmt.Errorf("coalesce scheduler item: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("coalesced scheduler item rows affected: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}
