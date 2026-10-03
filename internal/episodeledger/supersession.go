package episodeledger

import (
	"context"
	"database/sql"
	"fmt"
)

// SupersedeEpoch cancels in-flight episodes of a killed epoch (gate 2,
// first half): the runner's supersession watcher turns this into context
// cancellation of the provider call, and the decision gates (pre- and
// post-execute) refuse any outcome that still lands.
func SupersedeEpoch(ctx context.Context, tx *sql.Tx, epoch string, now string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE policy_epoch = ? AND lifecycle_status IN ('admitted', 'running')`,
		now, epoch); err != nil {
		return fmt.Errorf("supersede in-flight episodes of killed epoch: %w", err)
	}
	return nil
}

// SupersedeCoalesced cancels episodes and attempts bound to coalesced scheduler items.
func SupersedeCoalesced(ctx context.Context, tx *sql.Tx, situationID, now string) error {
	if _, err := tx.ExecContext(ctx, supersedeCoalescedEpisodesSQL,
		now, situationID,
	); err != nil {
		return fmt.Errorf("supersede episodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, cancelCoalescedAttemptsSQL,
		situationID,
	); err != nil {
		return fmt.Errorf("cancel superseded attempts: %w", err)
	}
	return nil
}

const supersedeCoalescedEpisodesSQL = `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE scheduler_item_id IN (
			SELECT scheduler_item_id FROM scheduler_items
			WHERE situation_id = ? AND status = 'coalesced'
		) AND lifecycle_status IN ('admitted', 'running')`

const cancelCoalescedAttemptsSQL = `
		UPDATE episode_attempts SET status = 'cancelling'
		WHERE episode_id IN (
			SELECT episode_id FROM episodes
			WHERE scheduler_item_id IN (
				SELECT scheduler_item_id FROM scheduler_items
				WHERE situation_id = ? AND status = 'coalesced'
			) AND lifecycle_status = 'superseded'
		) AND status IN ('dispatched', 'running')`
