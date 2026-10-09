package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func (t *Tx) UpsertSchedulerItem(ctx context.Context, item domain.SchedulerItem, tenantID string, dedupeKey []byte, now time.Time) error {
	if _, err := t.q.ExecContext(ctx, upsertSchedulerItemSQL, itemValues(item, tenantID, dedupeKey, now)...); err != nil {
		return fmt.Errorf("upsert scheduler item: %w", err)
	}
	return nil
}

func itemValues(item domain.SchedulerItem, tenantID string, dedupeKey []byte, now time.Time) []any {
	notBefore := sql.NullString{}
	if item.NotBefore != nil {
		notBefore = sql.NullString{String: kernel.FormatTime(*item.NotBefore), Valid: true}
	}
	return []any{
		item.SchedulerItemID, item.TriggerID, tenantID, item.SituationID, item.SituationVersion, item.Kind,
		item.Lane, item.Priority, item.Status, dedupeKey,
		notBefore, kernel.FormatTime(item.ExpiresAt), kernel.FormatTime(now), kernel.FormatTime(now),
	}
}

const upsertSchedulerItemSQL = `
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
