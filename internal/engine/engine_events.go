package engine

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (e *Engine) runGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	processed, lastPosition := 0, eventlog.LogPosition(0)
	for {
		batch, err := e.runGlobalBatch(ctx, lastPosition, beforeApply)
		processed += batch.processed
		if err != nil {
			return processed, err
		}
		if batch.empty {
			return e.finishGlobalRun(ctx, processed)
		}
		lastPosition = batch.lastPosition
	}
}

// globalBatch is the outcome of one page of the global event log.
type globalBatch struct {
	processed    int
	lastPosition eventlog.LogPosition
	empty        bool
}

// runGlobalBatch applies the next page after lastPosition and checkpoints
// the WAL; an empty page ends the run.
func (e *Engine) runGlobalBatch(ctx context.Context, lastPosition eventlog.LogPosition, beforeApply func(eventlog.Record) error) (globalBatch, error) {
	records, err := e.readGlobalRecords(ctx, lastPosition)
	if err != nil {
		return globalBatch{}, err
	}
	if len(records) == 0 {
		return globalBatch{empty: true}, nil
	}
	processed, position, err := e.applyGlobalBatch(ctx, records, beforeApply)
	if err != nil {
		return globalBatch{processed: processed}, err
	}
	if err := e.checkpointWAL(ctx); err != nil {
		return globalBatch{processed: processed}, fmt.Errorf("checkpoint WAL after global batch: %w", err)
	}
	return globalBatch{processed: processed, lastPosition: position}, nil
}

func (e *Engine) applyGlobalBatch(ctx context.Context, records []eventlog.Record, beforeApply func(eventlog.Record) error) (int, eventlog.LogPosition, error) {
	var processed int
	var lastPosition eventlog.LogPosition
	for _, record := range records {
		outcome, err := e.applyGlobalRecord(ctx, record, beforeApply)
		if err != nil {
			if outcome.timersRan {
				processed += outcome.fired
			}
			return processed, lastPosition, err
		}
		processed += outcome.fired + 1
		lastPosition = record.Position
	}
	return processed, lastPosition, nil
}

func (e *Engine) readGlobalRecords(ctx context.Context, afterPosition eventlog.LogPosition) ([]eventlog.Record, error) {
	var records []eventlog.Record
	if err := e.log.Read(ctx, eventlog.ReadRequest{
		TenantID: e.tenantID, PartitionID: -1, AfterPosition: afterPosition, Limit: 100,
	}, func(record eventlog.Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read global event log: %w", err)
	}
	return records, nil
}

func (e *Engine) finishGlobalRun(ctx context.Context, processed int) (int, error) {
	timerCount, err := e.runDueTimersForAllPartitions(ctx)
	if err != nil {
		return processed, fmt.Errorf("run global timers: %w", err)
	}
	if err := e.checkpointWAL(ctx); err != nil {
		return processed + timerCount, fmt.Errorf("checkpoint WAL after global timers: %w", err)
	}
	return processed + timerCount, nil
}

type globalRecordOutcome struct {
	fired     int
	timersRan bool
}

func (e *Engine) applyGlobalRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (globalRecordOutcome, error) {
	watermark, err := e.prepareRecord(ctx, record, beforeApply)
	if err != nil {
		return globalRecordOutcome{}, err
	}
	fired, err := e.runDueTimersForAllPartitions(ctx)
	if err != nil {
		return globalRecordOutcome{}, fmt.Errorf("run timers before event %d: %w", record.Position, err)
	}
	outcome := globalRecordOutcome{fired: fired, timersRan: true}
	if err := e.applyRecord(ctx, record.PartitionID, record, watermark); err != nil {
		return outcome, fmt.Errorf("apply record %d: %w", record.Position, err)
	}
	return outcome, nil
}

// prepareRecord derives the record's watermark from its partition checkpoint
// and runs the before-apply hook.
func (e *Engine) prepareRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (time.Time, error) {
	checkpoint, err := e.loadCheckpoint(ctx, record.PartitionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load partition checkpoint: %w", err)
	}
	watermark, err := e.watermarkForRecord(record.EventTime, checkpoint.Watermark)
	if err != nil {
		return time.Time{}, fmt.Errorf("watermark: %w", err)
	}
	if err := runBeforeApply(beforeApply, record); err != nil {
		return time.Time{}, err
	}
	return watermark, nil
}

func runBeforeApply(beforeApply func(eventlog.Record) error, record eventlog.Record) error {
	if beforeApply == nil {
		return nil
	}
	if err := beforeApply(record); err != nil {
		return fmt.Errorf("before apply hook: %w", err)
	}
	return nil
}

func (e *Engine) run(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	processed, err := e.drainPartition(ctx, partitionID, beforeApply)
	if err != nil {
		return processed, err
	}
	fired, err := e.runDueTimers(ctx, partitionID)
	if err != nil {
		return processed, err
	}
	processed += fired
	if err := e.checkpointWAL(ctx); err != nil {
		return processed, fmt.Errorf("checkpoint WAL after partition timers: %w", err)
	}
	return processed, nil
}

// drainPartition applies batches until the partition has no unread records.
func (e *Engine) drainPartition(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	processed := 0
	for {
		batchCount, err := e.runBatch(ctx, partitionID, beforeApply)
		if err != nil || batchCount == 0 {
			return processed, err
		}
		processed += batchCount
	}
}

func (e *Engine) runBatch(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	checkpoint, err := e.loadCheckpoint(ctx, partitionID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}
	records, err := e.readPartitionRecords(ctx, partitionID, checkpoint.LastPosition)
	if err != nil || len(records) == 0 {
		return 0, err
	}
	if err := e.applyPartitionRecords(ctx, partitionID, records, checkpoint, beforeApply); err != nil {
		return 0, err
	}
	if err := e.checkpointWAL(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint WAL after partition batch: %w", err)
	}
	return len(records), nil
}

// applyPartitionRecords applies records in log order, advancing the local
// watermark from the checkpoint record by record.
func (e *Engine) applyPartitionRecords(ctx context.Context, partitionID int, records []eventlog.Record, checkpoint checkpoint, beforeApply func(eventlog.Record) error) error {
	for _, record := range records {
		if err := runBeforeApply(beforeApply, record); err != nil {
			return err
		}
		watermark, err := e.watermarkForRecord(record.EventTime, checkpoint.Watermark)
		if err != nil {
			return fmt.Errorf("watermark: %w", err)
		}
		if err := e.applyRecord(ctx, partitionID, record, watermark); err != nil {
			return fmt.Errorf("apply record %d: %w", record.Position, err)
		}
		checkpoint.Watermark = watermark.Format(time.RFC3339Nano)
	}
	return nil
}

func (e *Engine) readPartitionRecords(ctx context.Context, partitionID int, afterPosition eventlog.LogPosition) ([]eventlog.Record, error) {
	const batchSize = 100
	var records []eventlog.Record
	if err := e.log.Read(ctx, eventlog.ReadRequest{
		TenantID: e.tenantID, PartitionID: partitionID, AfterPosition: afterPosition, Limit: batchSize,
	}, func(record eventlog.Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read event log: %w", err)
	}
	return records, nil
}

func (e *Engine) checkpointWAL(ctx context.Context) error {
	if err := e.db.Checkpoint(ctx); err != nil && !storage.IsSQLiteBusy(err) {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	return nil
}

type checkpoint struct {
	LastPosition eventlog.LogPosition
	Watermark    string
}

func (e *Engine) loadCheckpoint(ctx context.Context, partitionID int) (checkpoint, error) {
	var result checkpoint
	var lastPosition int64
	var watermark sql.NullString
	if err := e.db.QueryRowContext(ctx,
		"SELECT last_position, watermark FROM partition_checkpoints WHERE consumer_name = ? AND tenant_id = ? AND partition_id = ?",
		ConsumerName, e.tenantID, partitionID,
	).Scan(&lastPosition, &watermark); err != nil && err != sql.ErrNoRows {
		return result, fmt.Errorf("query checkpoint: %w", err)
	}
	result.LastPosition = eventlog.LogPosition(lastPosition)
	if watermark.Valid {
		result.Watermark = watermark.String
	}
	return result, nil
}

func (e *Engine) watermarkForRecord(eventTime time.Time, previousWatermark string) (time.Time, error) {
	maxLag, err := duration.Parse(e.spec.Time.MaxOutOfOrderness)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse maxOutOfOrderness: %w", err)
	}
	watermark := eventTime.Add(-maxLag)
	if previousWatermark == "" {
		return watermark, nil
	}
	previous, err := time.Parse(time.RFC3339Nano, previousWatermark)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse prev watermark: %w", err)
	}
	return latest(watermark, previous), nil
}

// latest keeps the watermark monotonic: it never moves before previous.
func latest(watermark, previous time.Time) time.Time {
	if watermark.Before(previous) {
		return previous
	}
	return watermark
}
