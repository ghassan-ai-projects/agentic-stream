package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func (s *Service) runDueTimersForAllPartitions(ctx context.Context) (int, error) {
	partitions, err := s.store.TimerPartitions(ctx)
	if err != nil {
		return 0, err //nolint:wrapcheck // The store names the failed read.
	}
	var fired int
	for _, partitionID := range partitions {
		partitionFired, err := s.runDueTimers(ctx, partitionID)
		if err != nil {
			return fired, err
		}
		fired += partitionFired
	}
	return fired, nil
}

func (s *Service) runDueTimers(ctx context.Context, partitionID int) (int, error) {
	now := s.clock.Now().UTC()
	watermark, err := s.timerWatermark(ctx, partitionID, now)
	if err != nil {
		return 0, err
	}
	fired := 0
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		fired, err = s.fireDueTimers(ctx, tx, partitionID, watermark, now)
		return err
	}); err != nil {
		return 0, s.timerFailure(ctx, err)
	}
	return fired, nil
}

func (s *Service) timerFailure(ctx context.Context, err error) error {
	if restoreErr := s.restoreAfterRollback(ctx); restoreErr != nil {
		return fmt.Errorf("run timers transaction: %w; restore after rollback: %w", err, restoreErr)
	}
	return fmt.Errorf("run timers transaction: %w", err)
}

func (s *Service) timerWatermark(ctx context.Context, partitionID int, now time.Time) (time.Time, error) {
	checkpoint, err := s.store.LoadCheckpoint(ctx, partitionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load timer checkpoint: %w", err)
	}
	return domain.TimerWatermark(checkpoint.Watermark, now) //nolint:wrapcheck // The domain rule names the failed parse.
}

// fireDueTimers applies the partition's due timers under the owner fence.
func (s *Service) fireDueTimers(ctx context.Context, tx *store.Tx, partitionID int, watermark, now time.Time) (int, error) {
	if err := tx.AssertOwner(ctx); err != nil {
		return 0, err
	}
	timers, err := tx.LoadDueTimers(ctx, partitionID, now)
	if err != nil || len(timers) == 0 {
		return 0, err
	}
	return s.applyDueTimers(ctx, tx, partitionID, timers, watermark, now)
}

func (s *Service) applyDueTimers(ctx context.Context, tx *store.Tx, partitionID int, timers []domain.DueTimer, watermark, now time.Time) (int, error) {
	state, features, err := s.timerFeatures(ctx, tx, partitionID, watermark, now)
	if err != nil {
		return 0, err
	}
	appliedFeatures, err := s.applyMatchedTimerFeatures(ctx, tx, partitionID, state, timers, features, watermark, now)
	if err != nil {
		return 0, err
	}
	if err := s.saveTimerSituationStates(ctx, tx, partitionID, appliedFeatures); err != nil {
		return 0, err
	}
	return len(timers), tx.AcknowledgeTimers(ctx, timers, now)
}

// timerFeatures loads the partition's operator state and lets the operators
// emit their timer features.
func (s *Service) timerFeatures(ctx context.Context, tx *store.Tx, partitionID int, watermark, now time.Time) (*operators.PartitionState, []operators.Feature, error) {
	state, err := tx.LoadOperatorState(ctx, partitionID, "")
	if err != nil {
		return nil, nil, err
	}
	features, _, err := s.opRuntime.ApplyTimer(ctx, state, watermark, now)
	if err != nil {
		return nil, nil, fmt.Errorf("apply timers: %w", err)
	}
	return state, features, nil
}

// applyMatchedTimerFeatures applies each feature that fires a due timer for its
// expected last event. Every timer must either fire or belong to a fenced
// (inactive) boot.
func (s *Service) applyMatchedTimerFeatures(ctx context.Context, tx *store.Tx, partitionID int, state *operators.PartitionState, timers []domain.DueTimer, features []operators.Feature, watermark, now time.Time) ([]operators.Feature, error) {
	matched := domain.InactiveTimerIDs(timers, func(stateKey string) bool { return s.opRuntime.IsTimerStateActive(state, stateKey) })
	var applied []operators.Feature
	for _, firing := range domain.MatchTimerFeatures(timers, features, matched) {
		feature, err := s.fireTimer(ctx, tx, partitionID, firing, watermark, now)
		if err != nil {
			return nil, err
		}
		applied = append(applied, feature)
	}
	return applied, domain.RequireAllTimersMatched(matched, timers) //nolint:wrapcheck // The domain rule names the unmatched timers.
}

// fireTimer records the firing's provenance and applies the feature.
func (s *Service) fireTimer(ctx context.Context, tx *store.Tx, partitionID int, firing domain.TimerFiring, watermark, now time.Time) (operators.Feature, error) {
	feature := firing.Feature
	domain.EnrichTimerFeature(&feature, s.tenantID, partitionID, firing.Timer, now, clock.Quality(s.clock))
	return feature, s.saveTimerFeature(ctx, tx, partitionID, feature, watermark)
}

func (s *Service) saveTimerFeature(ctx context.Context, tx *store.Tx, partitionID int, feature operators.Feature, watermark time.Time) error {
	versions, err := s.sitEngine.ApplyFeature(ctx, feature, watermark)
	if err != nil {
		return fmt.Errorf("apply timer situation: %w", err)
	}
	for _, version := range versions {
		if err := s.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
			return fmt.Errorf("save timer situation version: %w", err)
		}
		if s.cogEngine != nil {
			if err := tx.ProcessVersion(ctx, s.cogEngine, version); err != nil {
				return fmt.Errorf("process timer cognition: %w", err)
			}
		}
	}
	return nil
}

func (s *Service) saveTimerSituationStates(ctx context.Context, tx *store.Tx, partitionID int, features []operators.Feature) error {
	updated := make(map[string]struct{}, len(features))
	for _, feature := range features {
		key := domain.EntityKey(feature)
		if _, seen := updated[key]; seen {
			continue
		}
		updated[key] = struct{}{}
		if err := s.saveCurrentSituationState(ctx, tx, partitionID, feature.EntityType, feature.EntityID); err != nil {
			return err
		}
	}
	return nil
}

// scheduleHeartbeatTimers arms the missing-heartbeat timers the operator state
// implies, all stamped with one clock read.
func (s *Service) scheduleHeartbeatTimers(ctx context.Context, tx *store.Tx, partitionID int, state *operators.PartitionState) error {
	if state == nil {
		return nil
	}
	now := s.clock.Now().UTC()
	timers, err := domain.HeartbeatTimers(s.deploymentID, s.tenantID, partitionID, s.spec.Operators, state)
	if err != nil {
		return err //nolint:wrapcheck // The domain rule names the failed operator.
	}
	for _, timer := range timers {
		if err := tx.ArmHeartbeatTimer(ctx, partitionID, timer, now); err != nil {
			return err
		}
	}
	return nil
}
