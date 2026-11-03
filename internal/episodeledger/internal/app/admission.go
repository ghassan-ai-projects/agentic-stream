package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func Admit(ctx context.Context, tx *store.Tx, req domain.Admission, now time.Time) error {
	err := tx.InsertEpisode(ctx, req, now)
	if err == nil || !store.IsLiveEpisodeViolation(err) {
		return err
	}
	if req.ReportsLiveConflict() {
		return fmt.Errorf("%w: %w", domain.ErrLiveEpisodeConflict, err)
	}
	return fmt.Errorf("%w: %w", domain.ErrLiveEpisodeBusy, err)
}

func RecordRejection(ctx context.Context, tx *store.Tx, identity domain.Identity, reason domain.RejectionReason, details []byte, now time.Time) error {
	if err := domain.CheckRejectionReason(reason); err != nil {
		return err
	}
	details = domain.RejectionDetails(details)
	known, err := episodeKnown(ctx, tx, identity.EpisodeID)
	if err != nil {
		return err
	}
	return tx.InsertRejection(ctx, domain.Rejection{
		ID: domain.RejectionID(identity, reason, details, now), EpisodeID: identity.EpisodeID,
		AttemptID: identity.AttemptID, Fence: identity.Fence, Reason: reason, Details: details, At: now,
	}, known)
}

func episodeKnown(ctx context.Context, tx *store.Tx, episodeID string) (bool, error) {
	if episodeID == "" {
		return false, nil
	}
	return tx.EpisodeExists(ctx, episodeID)
}
