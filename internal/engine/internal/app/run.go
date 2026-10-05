package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

// RunGlobal applies all partitions in durable event-log position order. Replay
// uses it so one virtual clock cannot observe a later partition before an
// earlier record in the authoritative trace.
func (s *Service) RunGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runGlobal(ctx, beforeApply)
}

func (s *Service) runGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	processed, lastPosition := 0, eventlog.LogPosition(0)
	for {
		batch, err := s.runGlobalBatch(ctx, lastPosition, beforeApply)
		processed += batch.processed
		if err != nil {
			return processed, err
		}
		if batch.empty {
			return s.finishGlobalRun(ctx, processed)
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

// runGlobalBatch applies the next page after lastPosition and checkpoints the
// WAL; an empty page ends the run.
func (s *Service) runGlobalBatch(ctx context.Context, lastPosition eventlog.LogPosition, beforeApply func(eventlog.Record) error) (globalBatch, error) {
	records, err := s.readGlobalRecords(ctx, lastPosition)
	if err != nil {
		return globalBatch{}, err
	}
	if len(records) == 0 {
		return globalBatch{empty: true}, nil
	}
	processed, position, err := s.applyGlobalBatch(ctx, records, beforeApply)
	if err != nil {
		return globalBatch{processed: processed}, err
	}
	if err := s.store.CheckpointWAL(ctx); err != nil {
		return globalBatch{processed: processed}, fmt.Errorf("checkpoint WAL after global batch: %w", err)
	}
	return globalBatch{processed: processed, lastPosition: position}, nil
}

func (s *Service) applyGlobalBatch(ctx context.Context, records []eventlog.Record, beforeApply func(eventlog.Record) error) (int, eventlog.LogPosition, error) {
	var processed int
	var lastPosition eventlog.LogPosition
	for _, record := range records {
		outcome, err := s.applyGlobalRecord(ctx, record, beforeApply)
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

func (s *Service) readGlobalRecords(ctx context.Context, afterPosition eventlog.LogPosition) ([]eventlog.Record, error) {
	var records []eventlog.Record
	if err := s.log.Read(ctx, eventlog.ReadRequest{
		TenantID: s.tenantID, PartitionID: -1, AfterPosition: afterPosition, Limit: 100,
	}, func(record eventlog.Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read global event log: %w", err)
	}
	return records, nil
}

func (s *Service) finishGlobalRun(ctx context.Context, processed int) (int, error) {
	timerCount, err := s.runDueTimersForAllPartitions(ctx)
	if err != nil {
		return processed, fmt.Errorf("run global timers: %w", err)
	}
	if err := s.store.CheckpointWAL(ctx); err != nil {
		return processed + timerCount, fmt.Errorf("checkpoint WAL after global timers: %w", err)
	}
	return processed + timerCount, nil
}

type globalRecordOutcome struct {
	fired     int
	timersRan bool
}

func (s *Service) applyGlobalRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (globalRecordOutcome, error) {
	watermark, err := s.prepareRecord(ctx, record, beforeApply)
	if err != nil {
		return globalRecordOutcome{}, err
	}
	fired, err := s.runDueTimersForAllPartitions(ctx)
	if err != nil {
		return globalRecordOutcome{}, fmt.Errorf("run timers before event %d: %w", record.Position, err)
	}
	outcome := globalRecordOutcome{fired: fired, timersRan: true}
	if err := s.applyRecord(ctx, record.PartitionID, record, watermark); err != nil {
		return outcome, fmt.Errorf("apply record %d: %w", record.Position, err)
	}
	return outcome, nil
}

// prepareRecord derives the record's watermark from its partition checkpoint
// and runs the before-apply hook.
func (s *Service) prepareRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (time.Time, error) {
	checkpoint, err := s.store.LoadCheckpoint(ctx, record.PartitionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load partition checkpoint: %w", err)
	}
	watermark, err := s.watermarkForRecord(record.EventTime, checkpoint.Watermark)
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

func (s *Service) watermarkForRecord(eventTime time.Time, previous string) (time.Time, error) {
	return domain.WatermarkFor(eventTime, s.spec.Time.MaxOutOfOrderness, previous) //nolint:wrapcheck // Callers name the failed step.
}

func (s *Service) run(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	processed, err := s.drainPartition(ctx, partitionID, beforeApply)
	if err != nil {
		return processed, err
	}
	fired, err := s.runDueTimers(ctx, partitionID)
	if err != nil {
		return processed, err
	}
	processed += fired
	if err := s.store.CheckpointWAL(ctx); err != nil {
		return processed, fmt.Errorf("checkpoint WAL after partition timers: %w", err)
	}
	return processed, nil
}

// drainPartition applies batches until the partition has no unread records.
func (s *Service) drainPartition(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	processed := 0
	for {
		batchCount, err := s.runBatch(ctx, partitionID, beforeApply)
		if err != nil || batchCount == 0 {
			return processed, err
		}
		processed += batchCount
	}
}

func (s *Service) runBatch(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	checkpoint, err := s.store.LoadCheckpoint(ctx, partitionID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}
	records, err := s.readPartitionRecords(ctx, partitionID, eventlog.LogPosition(checkpoint.LastPosition))
	if err != nil || len(records) == 0 {
		return 0, err
	}
	if err := s.applyPartitionRecords(ctx, partitionID, records, checkpoint, beforeApply); err != nil {
		return 0, err
	}
	if err := s.store.CheckpointWAL(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint WAL after partition batch: %w", err)
	}
	return len(records), nil
}

// applyPartitionRecords applies records in log order, advancing the local
// watermark from the checkpoint record by record.
func (s *Service) applyPartitionRecords(ctx context.Context, partitionID int, records []eventlog.Record, checkpoint domain.Checkpoint, beforeApply func(eventlog.Record) error) error {
	for _, record := range records {
		if err := runBeforeApply(beforeApply, record); err != nil {
			return err
		}
		watermark, err := s.watermarkForRecord(record.EventTime, checkpoint.Watermark)
		if err != nil {
			return fmt.Errorf("watermark: %w", err)
		}
		if err := s.applyRecord(ctx, partitionID, record, watermark); err != nil {
			return fmt.Errorf("apply record %d: %w", record.Position, err)
		}
		checkpoint.Watermark = watermark.Format(time.RFC3339Nano)
	}
	return nil
}

func (s *Service) readPartitionRecords(ctx context.Context, partitionID int, afterPosition eventlog.LogPosition) ([]eventlog.Record, error) {
	const batchSize = 100
	var records []eventlog.Record
	if err := s.log.Read(ctx, eventlog.ReadRequest{
		TenantID: s.tenantID, PartitionID: partitionID, AfterPosition: afterPosition, Limit: batchSize,
	}, func(record eventlog.Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read event log: %w", err)
	}
	return records, nil
}
