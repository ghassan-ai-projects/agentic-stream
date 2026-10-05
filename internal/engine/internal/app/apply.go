package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func (s *Service) applyRecord(ctx context.Context, partitionID int, record eventlog.Record, watermark time.Time) error {
	err := s.store.RetryBusy(ctx, func() error {
		return s.applyRecordTransaction(ctx, partitionID, record, watermark)
	})
	if err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

func (s *Service) applyRecordTransaction(ctx context.Context, partitionID int, record eventlog.Record, watermark time.Time) error {
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.applyRecordInTx(ctx, tx, partitionID, record, watermark)
	})
	if err == nil {
		return nil
	}
	if restoreErr := s.restoreAfterRollback(ctx); restoreErr != nil {
		return fmt.Errorf("apply record transaction: %w; restore after rollback: %w", err, restoreErr)
	}
	return fmt.Errorf("apply transaction: %w", err)
}

func (s *Service) applyRecordInTx(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, watermark time.Time) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	applied, err := tx.EventApplied(ctx, record.EventID)
	if err != nil || applied {
		return err
	}
	newState, err := s.applyEventFeatures(ctx, tx, partitionID, record, watermark)
	if err != nil {
		return err
	}
	if err := s.persistOperatorState(ctx, tx, partitionID, record.Envelope.Entity.ID, newState); err != nil {
		return err
	}
	return tx.RecordApplied(ctx, partitionID, record.EventID, int64(record.Position), watermark, s.clock.Now().UTC())
}

// applyEventFeatures runs the operators over the event and folds every emitted
// feature into Situations, returning the new operator state.
func (s *Service) applyEventFeatures(ctx context.Context, tx *store.Tx, partitionID int, record eventlog.Record, watermark time.Time) (*operators.PartitionState, error) {
	operatorState, err := tx.LoadOperatorState(ctx, partitionID, record.Envelope.Entity.ID)
	if err != nil {
		return nil, fmt.Errorf("load operator state: %w", err)
	}
	features, newState, err := s.opRuntime.ApplyEventAt(ctx, operatorState, record.Envelope, watermark, s.clock.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("apply operators: %w", err)
	}
	if err := s.applyAndSaveFeatures(ctx, tx, partitionID, features, watermark); err != nil {
		return nil, err
	}
	return newState, nil
}

func (s *Service) applyAndSaveFeatures(ctx context.Context, tx *store.Tx, partitionID int, features []operators.Feature, watermark time.Time) error {
	affected, err := s.applyFeatures(ctx, tx, partitionID, features, watermark)
	if err != nil {
		return err
	}
	return s.saveAffectedSituationStates(ctx, tx, partitionID, affected)
}

// persistOperatorState saves the entity's operator state and re-arms its
// heartbeat timers.
func (s *Service) persistOperatorState(ctx context.Context, tx *store.Tx, partitionID int, entityID string, state *operators.PartitionState) error {
	if err := tx.SaveOperatorState(ctx, partitionID, entityID, state, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("save operator state: %w", err)
	}
	if err := s.scheduleHeartbeatTimers(ctx, tx, partitionID, state); err != nil {
		return fmt.Errorf("schedule heartbeat timers: %w", err)
	}
	return nil
}

func (s *Service) applyFeatures(ctx context.Context, tx *store.Tx, partitionID int, features []operators.Feature, watermark time.Time) (map[string]struct{}, error) {
	affected := make(map[string]struct{})
	for _, feature := range features {
		feature.TenantID = s.tenantID
		feature.PartitionID = partitionID
		affected[domain.EntityKey(feature)] = struct{}{}
		if err := s.applyFeature(ctx, tx, partitionID, feature, watermark); err != nil {
			return nil, err
		}
	}
	return affected, nil
}

// applyFeature updates the Situation and publishes each new version.
func (s *Service) applyFeature(ctx context.Context, tx *store.Tx, partitionID int, feature operators.Feature, watermark time.Time) error {
	versions, err := s.sitEngine.ApplyFeature(ctx, feature, watermark)
	if err != nil {
		return fmt.Errorf("apply situation: %w", err)
	}
	for _, version := range versions {
		if err := s.publishVersion(ctx, tx, partitionID, version); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) saveAffectedSituationStates(ctx context.Context, tx *store.Tx, partitionID int, affected map[string]struct{}) error {
	for key := range affected {
		parts := strings.SplitN(key, "\x00", 2)
		if err := s.saveCurrentSituationState(ctx, tx, partitionID, parts[0], parts[1]); err != nil {
			return err
		}
	}
	return nil
}

// saveCurrentSituationState persists the entity's in-memory Situation state
// once it has published a version.
func (s *Service) saveCurrentSituationState(ctx context.Context, tx *store.Tx, partitionID int, entityType, entityID string) error {
	situation, stateJSON, stateDigest, ok, err := s.sitEngine.CurrentState(partitionID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("snapshot current situation state: %w", err)
	}
	if !ok || situation.Version <= 0 {
		return nil
	}
	digest, err := domain.DecodeStateDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("save current situation state: %w", err)
	}
	if err := tx.SaveSituationRuntimeState(ctx, situation, stateJSON, digest, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("save current situation state: %w", err)
	}
	return nil
}
