package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EventApplied reports whether the inbox already holds the event.
func (tx *Tx) EventApplied(ctx context.Context, eventID string) (bool, error) {
	_, applied, err := storage.QueryOptional[int](ctx, tx.tx,
		"SELECT 1 FROM event_inbox WHERE consumer_name = ? AND tenant_id = ? AND event_id = ?",
		ConsumerName, tx.tenantID, eventID)
	if err != nil {
		return false, fmt.Errorf("check inbox: %w", err)
	}
	return applied, nil
}

// RecordApplied advances the partition checkpoint and marks the event applied.
func (tx *Tx) RecordApplied(ctx context.Context, partitionID int, eventID string, position int64, watermark, now time.Time) error {
	at := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO partition_checkpoints (consumer_name, tenant_id, partition_id, last_position, watermark, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(consumer_name, tenant_id, partition_id)
		DO UPDATE SET last_position = excluded.last_position, watermark = excluded.watermark, updated_at = excluded.updated_at`,
		ConsumerName, tx.tenantID, partitionID, position, watermark.Format(time.RFC3339Nano), at); err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx,
		"INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at) VALUES (?, ?, ?, ?, ?)",
		ConsumerName, tx.tenantID, eventID, position, at); err != nil {
		return fmt.Errorf("mark inbox: %w", err)
	}
	return nil
}
