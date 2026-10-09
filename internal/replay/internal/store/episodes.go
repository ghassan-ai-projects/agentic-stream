package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// MaterializeEpisodes turns every executable scheduler item into a durable
// episode through the episodes module's assembler, in one transaction.
func (s Store) MaterializeEpisodes(ctx context.Context, compiled *spec.CompiledSpec, tenantID string, now time.Time) error {
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: sources.Deterministic()})
	if err != nil {
		return fmt.Errorf("configure replay episodes: %w", err)
	}
	if err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return persistExecutableEpisodes(ctx, tx, assembler, tenantID, now)
	}); err != nil {
		return fmt.Errorf("materialize replay episodes: %w", err)
	}
	return nil
}

func persistExecutableEpisodes(ctx context.Context, tx *sql.Tx, assembler *episodes.Service, tenantID string, now time.Time) error {
	due, err := episodeledger.DueSchedulerItems(ctx, tx, tenantID, now)
	if err != nil {
		return fmt.Errorf("list due scheduler items: %w", err)
	}
	for _, item := range due {
		if err := persistReplayEpisode(ctx, tx, assembler, item, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func persistReplayEpisode(ctx context.Context, tx *sql.Tx, assembler *episodes.Service, item episodeledger.DueItem, tenantID string) error {
	req, err := assembler.Assemble(ctx, tx, item.SchedulerItemID, tenantID)
	if err != nil {
		return fmt.Errorf("assemble scheduler item %s: %w", item.SchedulerItemID, err)
	}
	if err := assembler.Persist(ctx, tx, req, item.AdmitAt); err != nil {
		return fmt.Errorf("persist replay episode %s: %w", req.EpisodeID, err)
	}
	return nil
}
