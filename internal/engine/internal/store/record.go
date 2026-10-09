package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (tx *Tx) EventApplied(ctx context.Context, eventID string) (bool, error) {
	_, applied, err := storage.QueryOptional[int](ctx, tx.tx,
		"SELECT 1 FROM event_inbox WHERE consumer_name = ? AND tenant_id = ? AND event_id = ?",
		ConsumerName, tx.tenantID, eventID)
	if err != nil {
		return false, fmt.Errorf("check inbox: %w", err)
	}
	return applied, nil
}

func (tx *Tx) RecordApplied(ctx context.Context, partitionID int, eventID string, position int64, clock domain.PartitionClock, now time.Time) error {
	sources, err := domain.EncodeSourceClocks(clock.Sources)
	if err != nil {
		return fmt.Errorf("checkpoint source clocks: %w", err)
	}
	at := kernel.FormatTime(now)
	if _, err := tx.tx.ExecContext(ctx, upsertCheckpointSQL,
		ConsumerName, tx.tenantID, partitionID, position, kernel.FormatTime(clock.Watermark), sources, at); err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx,
		"INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at) VALUES (?, ?, ?, ?, ?)",
		ConsumerName, tx.tenantID, eventID, position, at); err != nil {
		return fmt.Errorf("mark inbox: %w", err)
	}
	return nil
}

const upsertCheckpointSQL = `
		INSERT INTO partition_checkpoints (consumer_name, tenant_id, partition_id, last_position, watermark, sources_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(consumer_name, tenant_id, partition_id)
		DO UPDATE SET last_position = excluded.last_position, watermark = excluded.watermark,
		              sources_json = excluded.sources_json, updated_at = excluded.updated_at`

func (tx *Tx) RecordLateEvent(ctx context.Context, late domain.LateEvent, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO event_time_dispositions (
			tenant_id, event_id, log_position, partition_id, event_time, watermark, late_policy, disposition, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tx.tenantID, late.EventID, late.Position, late.PartitionID, kernel.FormatTime(late.EventTime),
		kernel.FormatTime(late.Watermark), late.Policy, string(late.Disposition), kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert late event: %w", err)
	}
	return nil
}

func (tx *Tx) RecordApplyFailure(ctx context.Context, failure domain.ApplyFailure, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO apply_failures (tenant_id, event_id, log_position, partition_id, step, error_text, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tx.tenantID, failure.EventID, failure.Position, failure.PartitionID, failure.Step, failure.ErrorText, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert apply failure: %w", err)
	}
	return nil
}
