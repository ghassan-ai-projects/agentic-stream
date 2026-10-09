package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// AdmitPendingSchedulerItem marks a pending item admitted and returns the rows it changed.
func (t *Tx) AdmitPendingSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'admitted', updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		kernel.FormatTime(now), schedulerItemID)
	if err != nil {
		return 0, fmt.Errorf("mark scheduler item admitted: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return count, nil
}

func (t *Tx) CoalesceTriggerItems(ctx context.Context, situationID, triggerName string, now time.Time) ([]domain.CoalescedItem, error) {
	items, err := storage.QueryAll(ctx, t.q, "coalesced scheduler items", scanCoalescedItem, `
		UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')
		RETURNING scheduler_item_id, situation_version`,
		kernel.FormatTime(now), situationID, situationID, triggerName,
	)
	if err != nil {
		return nil, fmt.Errorf("supersede scheduler items: %w", err)
	}
	slices.SortFunc(items, func(a, b domain.CoalescedItem) int { return cmp.Compare(a.SchedulerItemID, b.SchedulerItemID) })
	return items, nil
}

func scanCoalescedItem(rows *sql.Rows) (domain.CoalescedItem, error) {
	var item domain.CoalescedItem
	if err := rows.Scan(&item.SchedulerItemID, &item.SituationVersion); err != nil {
		return domain.CoalescedItem{}, fmt.Errorf("scan coalesced scheduler item: %w", err)
	}
	return item, nil
}

// CoalescePendingSchedulerItem removes one pending item from the queue and
// returns the rows it changed; operation labels errors.
func (t *Tx) CoalescePendingSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time, operation string) (int64, error) {
	return t.leavePending(ctx, schedulerItemID, now, "coalesced", operation)
}

func (t *Tx) leavePending(ctx context.Context, schedulerItemID string, now time.Time, status, operation string) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		UPDATE scheduler_items SET status = ?, updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		status, kernel.FormatTime(now), schedulerItemID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows affected: %w", operation, err)
	}
	return count, nil
}

func (t *Tx) PendingQueue(ctx context.Context, tenantID string) (domain.PendingQueue, error) {
	rows, err := storage.QueryAll(ctx, t.q, "pending scheduler items", scanQueueRow, `
		SELECT scheduler_item_id, created_at, not_before, expires_at
		FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'`, tenantID)
	if err != nil {
		return domain.PendingQueue{}, fmt.Errorf("find pending scheduler items: %w", err)
	}
	var queue domain.PendingQueue
	for _, row := range rows {
		queue.Add(row.parse())
	}
	return queue, nil
}

type queueRow struct {
	id, createdAt, expiresAt string
	notBefore                sql.NullString
}

func scanQueueRow(rows *sql.Rows) (queueRow, error) {
	var row queueRow
	if err := rows.Scan(&row.id, &row.createdAt, &row.notBefore, &row.expiresAt); err != nil {
		return queueRow{}, fmt.Errorf("scan pending scheduler item: %w", err)
	}
	return row, nil
}

func (r queueRow) parse() (domain.QueuedItem, string) {
	item := domain.QueuedItem{SchedulerItemID: r.id}
	var err error
	if item.CreatedAt, err = kernel.ParseTime(r.createdAt); err != nil {
		return item, "created_at"
	}
	if item.ExpiresAt, err = kernel.ParseTime(r.expiresAt); err != nil {
		return item, "expires_at"
	}
	return r.parseNotBefore(item)
}

func (r queueRow) parseNotBefore(item domain.QueuedItem) (domain.QueuedItem, string) {
	if !r.notBefore.Valid {
		return item, ""
	}
	notBefore, err := kernel.ParseTime(r.notBefore.String)
	if err != nil {
		return item, "not_before"
	}
	item.NotBefore = &notBefore
	return item, ""
}

func (t *Tx) ExpirePendingSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) (int64, error) {
	return t.leavePending(ctx, schedulerItemID, now, "expired", domain.OperationExpire)
}
