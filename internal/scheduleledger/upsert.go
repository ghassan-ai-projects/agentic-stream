package scheduleledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Upsert persists an admitted trigger opportunity and its deduplication identity.
func Upsert(ctx context.Context, tx *sql.Tx, item Item, tenantID string, dedupeKey []byte, now string) error {
	values := queueItemValues(item, tenantID, dedupeKey, now)
	return persistQueueItem(ctx, tx, values)
}

func queueItemValues(item Item, tenantID string, dedupeKey []byte, now string) []any {
	notBefore := sql.NullString{}
	if item.NotBefore != nil {
		notBefore = sql.NullString{String: item.NotBefore.Format(time.RFC3339Nano), Valid: true}
	}
	return []any{
		item.SchedulerItemID, item.TriggerID, tenantID, item.SituationID, item.SituationVersion, item.Kind,
		item.Lane, item.Priority, item.Status, dedupeKey,
		notBefore, item.ExpiresAt.Format(time.RFC3339Nano), now, now,
	}
}

func persistQueueItem(ctx context.Context, tx *sql.Tx, values []any) error {
	if _, err := tx.ExecContext(ctx, upsertQueueItemSQL, values...); err != nil {
		if isSchedulerItemIDConflict(err) {
			return retainExistingItemIdentity(ctx, tx, values)
		}
		return fmt.Errorf("upsert scheduler item: %w", err)
	}
	return nil
}

// upsertQueueItemSQL inserts a queue item or refreshes the item already
// queued for the same trigger.
const upsertQueueItemSQL = `
		INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
			kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id) DO UPDATE SET
			kind = excluded.kind,
			situation_version = excluded.situation_version,
			lane = excluded.lane,
			priority = excluded.priority,
			status = excluded.status,
			dedupe_key = excluded.dedupe_key,
			not_before = excluded.not_before,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`

func retainExistingItemIdentity(ctx context.Context, tx *sql.Tx, values []any) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
			kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scheduler_item_id) DO NOTHING`, values...); err != nil {
		return fmt.Errorf("ignore scheduler item ID conflict: %w", err)
	}
	return nil
}

func isSchedulerItemIDConflict(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqlite3.SQLITE_CONSTRAINT_UNIQUE:
		return strings.Contains(err.Error(), "UNIQUE constraint failed: scheduler_items.scheduler_item_id")
	default:
		return false
	}
}
