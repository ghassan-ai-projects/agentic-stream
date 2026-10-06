package episodeledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MarkSchedulerItemAdmitted requires the scheduler item to still be pending.
func MarkSchedulerItemAdmitted(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'admitted', updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.Format(time.RFC3339Nano), schedulerItemID,
	)
	if err != nil {
		return fmt.Errorf("mark scheduler item admitted: %w", err)
	}
	return requireStillPending(res, schedulerItemID, "rows affected")
}

// requireStillPending fails when a pending-only transition matched no item.
// The label prefixes a rows-affected error.
func requireStillPending(result sql.Result, schedulerItemID, label string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if n == 0 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}

// CoalesceSchedulerItems marks the trigger's open queue items replaced by newer work.
func CoalesceSchedulerItems(ctx context.Context, tx *sql.Tx, situationID, triggerName, now string) error {
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
	return nil
}
