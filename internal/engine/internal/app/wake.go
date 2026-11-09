package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (s *Service) WakeSituations(ctx context.Context, situationIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, situationID := range situationIDs {
		if err := s.wakeSituation(ctx, situationID, situations.WokenByWatch); err != nil {
			return fmt.Errorf("wake situation %s: %w", situationID, err)
		}
	}
	return nil
}

func (s *Service) wakeSituation(ctx context.Context, situationID, by string) error {
	partitionID, held := s.sitEngine.PartitionOf(situationID)
	if !held {
		return nil
	}
	checkpoint, err := s.store.LoadCheckpoint(ctx, partitionID)
	if err != nil {
		return fmt.Errorf("load checkpoint of partition %d: %w", partitionID, err)
	}
	err = s.store.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.AssertOwner(ctx); err != nil {
			return fmt.Errorf("assert owner before waking: %w", err)
		}
		return s.publishWokenVersion(ctx, tx, partitionID, situationID, by, checkpoint.Watermark)
	})
	if err == nil {
		return nil
	}
	if restoreErr := s.restoreAfterRollback(ctx); restoreErr != nil {
		return fmt.Errorf("%w; restore after rollback: %w", err, restoreErr)
	}
	return err
}

func (s *Service) publishWokenVersion(ctx context.Context, tx *store.Tx, partitionID int, situationID, by string, watermark time.Time) error {
	version, found, err := s.sitEngine.Woken(situationID, by, watermark)
	if err != nil {
		return fmt.Errorf("woken version: %w", err)
	}
	if !found {
		return nil
	}
	if err := s.publishVersion(ctx, tx, partitionID, *version); err != nil {
		return err
	}
	return s.saveCurrentSituationState(ctx, tx, partitionID, version.EntityType, version.EntityID)
}
