package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func ReadEpisodeFence(ctx context.Context, tx *store.Tx, episodeID string) (domain.EpisodeFence, bool, error) {
	return tx.ReadEpisodeFence(ctx, episodeID)
}

func ReadAttemptStatus(ctx context.Context, tx *store.Tx, identity domain.Identity) (domain.AttemptStatus, error) {
	record, err := attemptRecord(ctx, tx, identity)
	return record.Status, err
}

func ReadEpisodeLifecycle(ctx context.Context, tx *store.Tx, episodeID string) (domain.LifecycleStatus, error) {
	return tx.EpisodeLifecycle(ctx, episodeID)
}

func NextDispatchableEpisode(ctx context.Context, tx *store.Tx, tenantID string, killedSuperseded bool) (domain.DispatchableEpisode, bool, error) {
	return tx.NextDispatchableEpisode(ctx, tenantID, killedSuperseded)
}

func ReadAdmission(ctx context.Context, tx *store.Tx, episodeID string) (domain.Admission, error) {
	return tx.ReadAdmission(ctx, episodeID)
}
