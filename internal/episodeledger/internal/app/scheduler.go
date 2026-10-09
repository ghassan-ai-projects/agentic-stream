package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func UpsertSchedulerItem(ctx context.Context, tx *store.Tx, item domain.SchedulerItem, tenantID string, dedupeKey []byte, now time.Time) error {
	err := tx.UpsertSchedulerItem(ctx, item, tenantID, dedupeKey, now)
	if err != nil && store.IsSchedulerItemIDConflict(err) {
		return tx.InsertSchedulerItemIfAbsent(ctx, item, tenantID, dedupeKey, now)
	}
	return err
}

func MarkSchedulerItemAdmitted(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	rows, err := tx.AdmitPendingSchedulerItem(ctx, schedulerItemID, now)
	if err != nil {
		return err
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

func CoalesceSchedulerItems(ctx context.Context, tx *store.Tx, situationID, triggerName string, now time.Time) ([]domain.CoalescedItem, error) {
	return tx.CoalesceTriggerItems(ctx, situationID, triggerName, now)
}

func CoalesceCostRejectedItem(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	return coalescePending(ctx, tx, schedulerItemID, now, domain.OperationSkipCostRejected)
}

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

func ExpireSchedulerItem(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	rows, err := tx.ExpirePendingSchedulerItem(ctx, schedulerItemID, now)
	if err != nil {
		return err
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

func DueSchedulerItems(ctx context.Context, tx *store.Tx, tenantID string, now time.Time) ([]domain.DueItem, error) {
	queue, err := tx.PendingQueue(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return domain.DueItems(queue.Items, now), nil
}

func PollSchedulerQueue(ctx context.Context, tx *store.Tx, tenantID string, now time.Time) (domain.QueuePoll, error) {
	queue, err := tx.PendingQueue(ctx, tenantID)
	if err != nil {
		return domain.QueuePoll{}, err
	}
	return domain.Poll(queue, now), nil
}

func Scheduling(ctx context.Context, tx *store.Tx, tenantID, triggerID string) (domain.SchedulingRecord, bool, error) {
	return tx.Scheduling(ctx, tenantID, triggerID)
}

func Episode(ctx context.Context, tx *store.Tx, tenantID, episodeID string) (domain.EpisodeRecord, error) {
	return tx.Episode(ctx, tenantID, episodeID)
}
