package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

func (s *Service) RunGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runGlobal(ctx, beforeApply)
}

func (s *Service) runGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	lastPosition, err := s.appliedPosition(ctx)
	if err != nil {
		return 0, err
	}
	return s.applyUntilEmpty(ctx, lastPosition, beforeApply)
}

func (s *Service) applyUntilEmpty(ctx context.Context, lastPosition eventlog.LogPosition, beforeApply func(eventlog.Record) error) (int, error) {
	processed := 0
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

func (s *Service) appliedPosition(ctx context.Context) (eventlog.LogPosition, error) {
	applied, err := s.store.AppliedThrough(ctx)
	if err != nil {
		return 0, fmt.Errorf("read applied position: %w", err)
	}
	return eventlog.LogPosition(applied), nil
}

type globalBatch struct {
	processed    int
	lastPosition eventlog.LogPosition
	empty        bool
}

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
	return processed + timerCount, nil
}

type globalRecordOutcome struct {
	fired     int
	timersRan bool
}

func (s *Service) applyGlobalRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (globalRecordOutcome, error) {
	prepared, err := s.prepareRecord(ctx, record, beforeApply)
	if err != nil {
		return globalRecordOutcome{}, err
	}
	fired, err := s.runDueTimersForAllPartitions(ctx)
	if err != nil {
		return globalRecordOutcome{}, fmt.Errorf("run timers before event %d: %w", record.Position, err)
	}
	outcome := globalRecordOutcome{fired: fired, timersRan: true}
	if err := s.applyRecord(ctx, record.PartitionID, record, prepared); err != nil {
		return outcome, fmt.Errorf("apply record %d: %w", record.Position, err)
	}
	return outcome, nil
}

type preparedRecord struct {
	clock    domain.PartitionClock
	lateness domain.LateDisposition
}

func (s *Service) prepareRecord(ctx context.Context, record eventlog.Record, beforeApply func(eventlog.Record) error) (preparedRecord, error) {
	checkpoint, err := s.store.LoadCheckpoint(ctx, record.PartitionID)
	if err != nil {
		return preparedRecord{}, fmt.Errorf("load partition checkpoint: %w", err)
	}
	clock := domain.EventClock{Source: record.Envelope.Source, EventTime: record.EventTime, IngestedAt: record.Envelope.IngestedAt}
	placement, err := domain.PlaceInTime(clock, checkpoint, s.spec.Time)
	if err != nil {
		return preparedRecord{}, fmt.Errorf("place event %s in time: %w", record.EventID, err)
	}
	if err := runBeforeApply(beforeApply, record); err != nil {
		return preparedRecord{}, err
	}
	return preparedRecord{clock: placement.Clock, lateness: placement.Disposition}, nil
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
