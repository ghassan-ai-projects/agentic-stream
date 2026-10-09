package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (s *Service) applyRecord(ctx context.Context, partitionID int, record eventlog.Record, prepared preparedRecord) error {
	err := s.store.RetryBusy(ctx, func() error {
		return s.applyRecordTransaction(ctx, partitionID, record, prepared)
	})
	if failure, ruled := domain.AsRuleFailure(err); ruled {
		return s.setAsideFailedRecord(ctx, partitionID, record, prepared, failure)
	}
	if err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

func (s *Service) setAsideFailedRecord(ctx context.Context, partitionID int, record eventlog.Record, prepared preparedRecord, failure *domain.RuleFailure) error {
	s.logger.Error("event set aside: applying it fails every time", "event_id", record.EventID, "position", record.Position, "step", failure.Step, "error", failure.Err)
	setAside := domain.ApplyFailure{PartitionID: partitionID, EventID: record.EventID, Position: int64(record.Position), Step: failure.Step, ErrorText: failure.Err.Error()}
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		if applied, err := s.alreadyApplied(ctx, tx, record.EventID); err != nil || applied {
			return err
		}
		if err := tx.RecordApplyFailure(ctx, setAside, s.clock.Now().UTC()); err != nil {
			return fmt.Errorf("record apply failure of %s: %w", record.EventID, err)
		}
		return tx.RecordApplied(ctx, partitionID, record.EventID, int64(record.Position), prepared.clock, s.clock.Now().UTC())
	})
	if err != nil {
		return fmt.Errorf("set aside event %s: %w", record.EventID, err)
	}
	return nil
}

func (s *Service) applyRecordTransaction(ctx context.Context, partitionID int, record eventlog.Record, prepared preparedRecord) error {
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.applyRecordInTx(ctx, tx, partitionID, record, prepared)
	})
	if err == nil {
		return nil
	}
	if restoreErr := s.restoreAfterRollback(ctx); restoreErr != nil {
		return fmt.Errorf("%w; restore after rollback: %w", err, restoreErr)
	}
	return err
}

func (s *Service) applyRecordInTx(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, prepared preparedRecord) error {
	if applied, err := s.alreadyApplied(ctx, tx, record.EventID); err != nil || applied {
		return err
	}
	if err := s.applyEventState(ctx, tx, partitionID, record, prepared); err != nil {
		return err
	}
	if err := tx.RecordApplied(ctx, partitionID, record.EventID, int64(record.Position), prepared.clock, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("record event %s applied: %w", record.EventID, err)
	}
	return nil
}

func (s *Service) alreadyApplied(ctx context.Context, tx *store.Tx, eventID string) (bool, error) {
	if err := tx.AssertOwner(ctx); err != nil {
		return false, fmt.Errorf("assert owner before event %s: %w", eventID, err)
	}
	applied, err := tx.EventApplied(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("check inbox for event %s: %w", eventID, err)
	}
	return applied, nil
}

func (s *Service) applyEventState(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, prepared preparedRecord) error {
	if err := s.recordLateness(ctx, tx, partitionID, record, prepared); err != nil {
		return err
	}
	if !prepared.lateness.ChangesState() {
		return nil
	}
	newState, err := s.applyEventFeatures(ctx, tx, partitionID, record, prepared.clock.Watermark)
	if err != nil {
		return err
	}
	return s.persistOperatorState(ctx, tx, partitionID, record.Envelope.Entity.ID, newState)
}

func (s *Service) recordLateness(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, prepared preparedRecord) error {
	if prepared.lateness == domain.OnTime {
		return nil
	}
	late := domain.LateEvent{
		PartitionID: partitionID, EventID: record.EventID, Position: int64(record.Position),
		EventTime: record.EventTime, Watermark: prepared.clock.Watermark,
		Policy: s.spec.Time.LatePolicy, Disposition: prepared.lateness,
	}
	if err := tx.RecordLateEvent(ctx, late, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("record late event %s: %w", record.EventID, err)
	}
	return nil
}

func (s *Service) applyEventFeatures(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, watermark time.Time) (*operators.PartitionState, error) {
	operatorState, err := tx.LoadOperatorState(ctx, partitionID, record.Envelope.Entity.ID)
	if err != nil {
		return nil, fmt.Errorf("load operator state: %w", err)
	}
	features, newState, err := s.opRuntime.ApplyEventAt(ctx, operatorState, record.Envelope, watermark, s.clock.Now().UTC())
	if err != nil {
		return nil, ruleFailure(ctx, "apply operators", err)
	}
	if err := s.applyAndSaveFeatures(ctx, tx, partitionID, features, watermark); err != nil {
		return nil, err
	}
	return newState, nil
}

func (s *Service) applyAndSaveFeatures(ctx context.Context, tx *store.Tx, partitionID int, features []operators.Feature, watermark time.Time) error {
	for _, feature := range features {
		feature.TenantID = s.tenantID
		feature.PartitionID = partitionID
		if err := s.applyFeature(ctx, tx, partitionID, feature, watermark); err != nil {
			return err
		}
	}
	return s.saveSituationStatesOf(ctx, tx, partitionID, features)
}

func (s *Service) persistOperatorState(ctx context.Context, tx *store.Tx, partitionID int, entityID string, state *operators.PartitionState) error {
	if err := tx.SaveOperatorState(ctx, partitionID, entityID, state, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("save operator state: %w", err)
	}
	if err := s.scheduleHeartbeatTimers(ctx, tx, partitionID, state); err != nil {
		return fmt.Errorf("schedule heartbeat timers: %w", err)
	}
	return nil
}

func (s *Service) applyFeature(ctx context.Context, tx *store.Tx, partitionID int, feature operators.Feature, watermark time.Time) error {
	versions, err := s.sitEngine.ApplyFeature(ctx, feature, watermark)
	if err != nil {
		return ruleFailure(ctx, "apply situation", err)
	}
	for _, version := range versions {
		if err := s.publishVersion(ctx, tx, partitionID, version); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) saveSituationStatesOf(ctx context.Context, tx *store.Tx, partitionID int, features []operators.Feature) error {
	for _, entity := range domain.DistinctEntities(features) {
		if err := s.saveCurrentSituationState(ctx, tx, partitionID, entity.Type, entity.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) saveCurrentSituationState(ctx context.Context, tx *store.Tx, partitionID int, entityType, entityID string) error {
	situation, stateJSON, stateDigest, ok, err := s.sitEngine.CurrentState(partitionID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("snapshot current situation state: %w", err)
	}
	if !ok {
		return nil
	}
	digest, err := domain.DecodeStateDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("save current situation state: %w", err)
	}
	if err := s.saveSituationState(ctx, tx, situation, stateJSON, digest); err != nil {
		return fmt.Errorf("save state of situation %s: %w", situation.SituationID, err)
	}
	return nil
}

func (s *Service) saveSituationState(ctx context.Context, tx *store.Tx, situation situations.Situation, stateJSON, digest []byte) error {
	if situation.Version == 0 {
		return tx.SaveUnopenedSituationState(ctx, situation, stateJSON, digest, s.clock.Now().UTC())
	}
	return tx.SaveSituationRuntimeState(ctx, situation, stateJSON, digest, s.clock.Now().UTC())
}

func ruleFailure(ctx context.Context, step string, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", step, err)
	}
	return &domain.RuleFailure{Step: step, Err: err}
}
