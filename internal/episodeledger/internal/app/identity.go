package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// ValidateWorkerIdentity validates a worker identity against the current
// episode fence, the owner lease and the attempt state. Snapshot equality is
// intentionally not part of this check. now is the instant the owner lease is
// judged at.
func ValidateWorkerIdentity(ctx context.Context, tx *store.Tx, identity domain.Identity, now time.Time) error {
	if err := checkEpisodeFence(ctx, tx, identity); err != nil {
		return err
	}
	if err := requireOwnerLease(ctx, tx, identity.OwnerEpoch, now); err != nil {
		return err
	}
	return checkAttemptOpen(ctx, tx, identity)
}

// checkEpisodeFence requires an open episode whose current attempt and fence
// are exactly the identity's; an older fence is stale.
func checkEpisodeFence(ctx context.Context, tx *store.Tx, identity domain.Identity) error {
	fence, err := episodeFence(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	return fence.CheckOpenIdentity(identity)
}

func episodeFence(ctx context.Context, tx *store.Tx, episodeID string) (domain.EpisodeFence, error) {
	fence, found, err := tx.ReadEpisodeFence(ctx, episodeID)
	if err != nil {
		return domain.EpisodeFence{}, err
	}
	if !found {
		return domain.EpisodeFence{}, domain.Refuse(domain.RejectUnknownEpisode)
	}
	return fence, nil
}

// requireOwnerLease refuses an identity whose owner epoch no longer holds an
// unexpired runtime lease. An identity without an owner epoch is not fenced.
func requireOwnerLease(ctx context.Context, tx *store.Tx, ownerEpoch string, now time.Time) error {
	if ownerEpoch == "" {
		return nil
	}
	held, err := tx.OwnerHoldsLease(ctx, ownerEpoch, now)
	if err != nil {
		return err
	}
	if !held {
		return domain.Refuse(domain.RejectStaleAttempt)
	}
	return nil
}

// checkAttemptOpen requires the attempt row to exist under the identity's
// owner epoch and not be terminal.
func checkAttemptOpen(ctx context.Context, tx *store.Tx, identity domain.Identity) error {
	record, err := attemptRecord(ctx, tx, identity)
	if err != nil {
		return err
	}
	return domain.CheckAttemptOpen(identity, record)
}

func attemptRecord(ctx context.Context, tx *store.Tx, identity domain.Identity) (domain.AttemptRecord, error) {
	record, found, err := tx.ReadAttempt(ctx, identity)
	if err != nil {
		return domain.AttemptRecord{}, err
	}
	if !found {
		return domain.AttemptRecord{}, domain.Refuse(domain.RejectWrongAttempt)
	}
	return record, nil
}

// validateTerminalIdentity validates the acknowledgement of a cancellation on a
// closed episode: the fence, the owner lease and an attempt that is not yet terminal.
func validateTerminalIdentity(ctx context.Context, tx *store.Tx, identity domain.Identity, now time.Time) error {
	if err := checkTerminalFence(ctx, tx, identity); err != nil {
		return err
	}
	if err := requireOwnerLease(ctx, tx, identity.OwnerEpoch, now); err != nil {
		return err
	}
	record, err := attemptRecord(ctx, tx, identity)
	if err != nil {
		return err
	}
	return domain.CheckAttemptNotTerminal(record.Status)
}

func checkTerminalFence(ctx context.Context, tx *store.Tx, identity domain.Identity) error {
	fence, err := episodeFence(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	return fence.CheckIdentity(identity)
}
