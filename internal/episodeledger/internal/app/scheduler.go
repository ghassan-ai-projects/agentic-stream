package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// UpsertSchedulerItem persists an admitted trigger opportunity and its
// deduplication identity. A clash on the item id keeps the existing item.
func UpsertSchedulerItem(ctx context.Context, tx *store.Tx, item domain.SchedulerItem, tenantID string, dedupeKey []byte, now string) error {
	err := tx.UpsertSchedulerItem(ctx, item, tenantID, dedupeKey, now)
	if err != nil && store.IsSchedulerItemIDConflict(err) {
		return tx.InsertSchedulerItemIfAbsent(ctx, item, tenantID, dedupeKey, now)
	}
	return err
}

// MarkSchedulerItemAdmitted requires the scheduler item to still be pending.
func MarkSchedulerItemAdmitted(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	rows, err := tx.AdmitPendingSchedulerItem(ctx, schedulerItemID, now)
	if err != nil {
		return err
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

// CoalesceSchedulerItems marks the trigger's open queue items replaced by newer work.
func CoalesceSchedulerItems(ctx context.Context, tx *store.Tx, situationID, triggerName, now string) error {
	return tx.CoalesceTriggerItems(ctx, situationID, triggerName, now)
}

// CoalesceCostRejectedItem prevents a cost-rejected opportunity from blocking the queue.
func CoalesceCostRejectedItem(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	return coalescePending(ctx, tx, schedulerItemID, now, domain.OperationSkipCostRejected)
}

// CoalesceSkippedItem removes an unavailable opportunity from the pending queue.
func CoalesceSkippedItem(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	return coalescePending(ctx, tx, schedulerItemID, now, domain.OperationCoalesceSkipped)
}

func coalescePending(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time, operation string) error {
	rows, err := tx.CoalescePendingSchedulerItem(ctx, schedulerItemID, now, operation)
	if err != nil {
		return err
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

// NextPendingSchedulerItem returns the tenant's next pending scheduler item
// that is due at now, in queue order.
func NextPendingSchedulerItem(ctx context.Context, tx *store.Tx, tenantID string, now time.Time) (string, bool, error) {
	return tx.NextPendingSchedulerItem(ctx, tenantID, now)
}
