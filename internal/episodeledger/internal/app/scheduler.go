package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func UpsertSchedulerItem(ctx context.Context, tx *store.Tx, item domain.SchedulerItem, tenantID string, dedupeKey []byte, now time.Time) error {
	if err := tx.UpsertSchedulerItem(ctx, item, tenantID, dedupeKey, now); err != nil {
		return fmt.Errorf("upsert scheduler item %s: %w", item.SchedulerItemID, err)
	}
	return nil
}

func MarkSchedulerItemAdmitted(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	rows, err := tx.AdmitPendingSchedulerItem(ctx, schedulerItemID, now)
	if err != nil {
		return fmt.Errorf("admit scheduler item %s: %w", schedulerItemID, err)
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

func CoalesceSchedulerItems(ctx context.Context, tx *store.Tx, situationID, triggerName string, now time.Time) ([]domain.CoalescedItem, error) {
	items, err := tx.CoalesceTriggerItems(ctx, situationID, triggerName, now)
	if err != nil {
		return nil, fmt.Errorf("coalesce %s items of situation %s: %w", triggerName, situationID, err)
	}
	return items, nil
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
		return fmt.Errorf("coalesce scheduler item %s: %w", schedulerItemID, err)
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

func ExpireSchedulerItem(ctx context.Context, tx *store.Tx, schedulerItemID string, now time.Time) error {
	rows, err := tx.ExpirePendingSchedulerItem(ctx, schedulerItemID, now)
	if err != nil {
		return fmt.Errorf("expire scheduler item %s: %w", schedulerItemID, err)
	}
	return domain.CheckStillPending(rows, schedulerItemID)
}

func DueSchedulerItems(ctx context.Context, tx *store.Tx, tenantID string, now time.Time) ([]domain.DueItem, error) {
	queue, err := tx.PendingQueue(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read pending queue: %w", err)
	}
	return domain.DueItems(queue.Items, now), nil
}

func PollSchedulerQueue(ctx context.Context, tx *store.Tx, tenantID string, now time.Time) (domain.QueuePoll, error) {
	queue, err := tx.PendingQueue(ctx, tenantID)
	if err != nil {
		return domain.QueuePoll{}, fmt.Errorf("read pending queue: %w", err)
	}
	return domain.Poll(queue, now), nil
}

func Scheduling(ctx context.Context, tx *store.Tx, tenantID, triggerID string) (domain.SchedulingRecord, bool, error) {
	record, found, err := tx.Scheduling(ctx, tenantID, triggerID)
	if err != nil {
		return domain.SchedulingRecord{}, false, fmt.Errorf("read scheduling of trigger %s: %w", triggerID, err)
	}
	return record, found, nil
}

func Episode(ctx context.Context, tx *store.Tx, tenantID, episodeID string) (domain.EpisodeRecord, error) {
	episode, err := tx.Episode(ctx, tenantID, episodeID)
	if err != nil {
		return domain.EpisodeRecord{}, fmt.Errorf("read episode %s: %w", episodeID, err)
	}
	return episode, nil
}
