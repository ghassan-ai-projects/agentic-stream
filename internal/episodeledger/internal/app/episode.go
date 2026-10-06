package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// Rebind persists the validated live snapshot and consumes one rebind.
func Rebind(ctx context.Context, tx *store.Tx, episodeID string, version int, digest, request []byte) error {
	return tx.RebindEpisode(ctx, episodeID, version, digest, request)
}

// BindRequest persists the request after a fenced attempt identity is bound.
func BindRequest(ctx context.Context, tx *store.Tx, episodeID string, request []byte) error {
	return tx.BindRequest(ctx, episodeID, request)
}

// AbandonRebind quarantines an invalid live snapshot and consumes one rebind.
func AbandonRebind(ctx context.Context, tx *store.Tx, episodeID, now string, terminal []byte) error {
	return tx.AbandonRebind(ctx, episodeID, now, terminal)
}

// Abandon records a terminal quarantine outcome.
func Abandon(ctx context.Context, tx *store.Tx, episodeID, now string, terminal []byte) error {
	return tx.AbandonEpisode(ctx, episodeID, now, terminal)
}

// Conclude records the terminal execution outcome.
func Conclude(ctx context.Context, tx *store.Tx, episodeID, now string, terminal []byte) error {
	return tx.ConcludeEpisode(ctx, episodeID, now, terminal)
}

// RetainForRetry retains the episode for the next bounded attempt.
func RetainForRetry(ctx context.Context, tx *store.Tx, episodeID string) error {
	return tx.RetainEpisodeForRetry(ctx, episodeID)
}

// SupersedeEpoch cancels in-flight episodes of a killed epoch: the runner's
// supersession watcher turns this into cancellation of the provider call, and
// the decision gates refuse any outcome that still lands.
func SupersedeEpoch(ctx context.Context, tx *store.Tx, epoch, now string) error {
	return tx.SupersedeEpochEpisodes(ctx, epoch, now)
}

// SupersedeCoalesced cancels episodes and attempts bound to coalesced scheduler items.
func SupersedeCoalesced(ctx context.Context, tx *store.Tx, situationID, now string) error {
	if err := tx.SupersedeCoalescedEpisodes(ctx, situationID, now); err != nil {
		return err
	}
	return tx.CancelCoalescedAttempts(ctx, situationID)
}
