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

// CoalesceTriggerItems marks the trigger's open queue items replaced by newer
// work and returns them in identity order.
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
	result, err := t.q.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
		kernel.FormatTime(now), schedulerItemID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows affected: %w", operation, err)
	}
	return count, nil
}

// PendingQueuedItems lists the tenant's pending scheduler items with their
// admission times.
func (t *Tx) PendingQueuedItems(ctx context.Context, tenantID string) ([]domain.QueuedItem, error) {
	items, err := storage.QueryAll(ctx, t.q, "pending scheduler items", scanQueuedItem, `
		SELECT scheduler_item_id, created_at, not_before, expires_at
		FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find pending scheduler items: %w", err)
	}
	return items, nil
}

func scanQueuedItem(rows *sql.Rows) (domain.QueuedItem, error) {
	var id, createdAt, expiresAt string
	var notBefore sql.NullString
	if err := rows.Scan(&id, &createdAt, &notBefore, &expiresAt); err != nil {
		return domain.QueuedItem{}, fmt.Errorf("scan pending scheduler item: %w", err)
	}
	return parseQueuedItem(id, createdAt, notBefore, expiresAt)
}

func parseQueuedItem(id, createdAt string, notBefore sql.NullString, expiresAt string) (domain.QueuedItem, error) {
	item := domain.QueuedItem{SchedulerItemID: id}
	var err error
	if item.CreatedAt, err = parseQueueTime(id, "created_at", createdAt); err != nil {
		return domain.QueuedItem{}, err
	}
	if item.ExpiresAt, err = parseQueueTime(id, "expires_at", expiresAt); err != nil {
		return domain.QueuedItem{}, err
	}
	if notBefore.Valid {
		parsed, err := parseQueueTime(id, "not_before", notBefore.String)
		item.NotBefore = &parsed
		return item, err
	}
	return item, nil
}

func parseQueueTime(id, column, text string) (time.Time, error) {
	parsed, err := kernel.ParseTime(text)
	if err != nil {
		return time.Time{}, fmt.Errorf("scheduler item %s %s: %w", id, column, err)
	}
	return parsed, nil
}
