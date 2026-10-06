package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// StartAttempt allocates the next fence for an admitted episode and records a
// dispatched worker attempt on the caller's transaction.
func StartAttempt(ctx context.Context, tx *store.Tx, episodeID, attemptID string, now time.Time) (domain.Identity, error) {
	return startAttempt(ctx, tx, episodeID, attemptID, "", now)
}

// StartAttemptOwned allocates an attempt fenced to the current runtime epoch.
// Live composition uses this entry point; StartAttempt serves isolated fixtures
// that do not model runtime ownership.
func StartAttemptOwned(ctx context.Context, tx *store.Tx, episodeID, attemptID, ownerEpoch string, now time.Time) (domain.Identity, error) {
	if ownerEpoch == "" {
		return domain.Identity{}, fmt.Errorf("runtime owner epoch is required")
	}
	return startAttempt(ctx, tx, episodeID, attemptID, ownerEpoch, now)
}

func startAttempt(ctx context.Context, tx *store.Tx, episodeID, attemptID, ownerEpoch string, now time.Time) (domain.Identity, error) {
	if episodeID == "" || attemptID == "" {
		return domain.Identity{}, fmt.Errorf("episode and attempt IDs are required")
	}
	if err := requireOwnerLease(ctx, tx, ownerEpoch, now); err != nil {
		return domain.Identity{}, err
	}
	fence, err := requireStartableEpisode(ctx, tx, episodeID)
	if err != nil {
		return domain.Identity{}, err
	}
	identity := domain.Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: fence + 1, OwnerEpoch: ownerEpoch}
	if err := recordStartedAttempt(ctx, tx, identity, now); err != nil {
		return domain.Identity{}, err
	}
	return identity, nil
}

// requireStartableEpisode requires an admitted or running episode with no
// active attempt and returns its current fence.
func requireStartableEpisode(ctx context.Context, tx *store.Tx, episodeID string) (int64, error) {
	fence, err := episodeFence(ctx, tx, episodeID)
	if err != nil {
		return 0, err
	}
	current, err := fence.CheckStartable()
	if err != nil || !fence.HasAttempt {
		return current, err
	}
	status, err := tx.ReadEpisodeAttemptStatus(ctx, episodeID, fence.Attempt)
	if err != nil {
		return 0, err
	}
	return current, domain.CheckPriorAttemptTerminal(episodeID, fence.Attempt, status)
}

func recordStartedAttempt(ctx context.Context, tx *store.Tx, identity domain.Identity, now time.Time) error {
	startedAt := store.TimeText(now)
	if err := tx.InsertAttempt(ctx, identity, startedAt); err != nil {
		return err
	}
	return tx.RecordEpisodeAttempt(ctx, identity, startedAt)
}
