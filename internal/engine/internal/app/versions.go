package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

// publishVersion persists the version and lets cognition react to it in the
// same transaction.
func (s *Service) publishVersion(ctx context.Context, tx *store.Tx, partitionID int, version situations.Version) error {
	if err := s.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
		return fmt.Errorf("save situation version: %w", err)
	}
	if s.cogEngine == nil {
		return nil
	}
	if err := tx.ProcessVersion(ctx, s.cogEngine, version); err != nil {
		return fmt.Errorf("cognition process: %w", err)
	}
	return nil
}

// saveSituationVersion writes the lineage, the Situation's current row and the
// immutable version row, all stamped with one clock read.
func (s *Service) saveSituationVersion(ctx context.Context, tx *store.Tx, partitionID int, version situations.Version) error {
	now := s.clock.Now().UTC()
	write, err := domain.NewSituationWrite(version)
	if err != nil {
		return err //nolint:wrapcheck // The domain rule names the failed version.
	}
	lineage, err := domain.NewLineage(version.Evidence)
	if err != nil {
		return err //nolint:wrapcheck // The domain rule names the failed evidence.
	}
	if err := tx.RecordLineage(ctx, lineage, now); err != nil {
		return err
	}
	if err := tx.UpsertSituation(ctx, partitionID, version, write, now); err != nil {
		return err
	}
	return tx.InsertSituationVersion(ctx, version, lineage.ID, now)
}
