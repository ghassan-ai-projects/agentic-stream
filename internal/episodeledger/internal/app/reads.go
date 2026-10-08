package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// ReadEpisodeFence reads the episode's tenant, lifecycle, current attempt and
// fence; found is false for an unknown episode.
func ReadEpisodeFence(ctx context.Context, tx *store.Tx, episodeID string) (domain.EpisodeFence, bool, error) {
	return tx.ReadEpisodeFence(ctx, episodeID)
}

// ReadAttemptStatus reads the status of the attempt under the identity; a
// missing attempt is refused as the wrong attempt.
func ReadAttemptStatus(ctx context.Context, tx *store.Tx, identity domain.Identity) (domain.AttemptStatus, error) {
	record, err := attemptRecord(ctx, tx, identity)
	return record.Status, err
}

// ReadEpisodeLifecycle reads the episode's lifecycle status; an unknown
// episode is an error.
func ReadEpisodeLifecycle(ctx context.Context, tx *store.Tx, episodeID string) (domain.LifecycleStatus, error) {
	return tx.EpisodeLifecycle(ctx, episodeID)
}

// NextDispatchableEpisode reads the tenant's oldest dispatchable episode;
// found is false when nothing is dispatchable.
func NextDispatchableEpisode(ctx context.Context, tx *store.Tx, tenantID string, killedSuperseded bool) (domain.DispatchableEpisode, bool, error) {
	return tx.NextDispatchableEpisode(ctx, tenantID, killedSuperseded)
}

// ReadAdmission reads the admission record of an episode.
func ReadAdmission(ctx context.Context, tx *store.Tx, episodeID string) (domain.Admission, error) {
	return tx.ReadAdmission(ctx, episodeID)
}
