package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// TransitionAttempt applies a valid attempt transition and records terminal
// data. The identity is checked before the state mutation; wall is the instant
// the owner lease is judged at.
func TransitionAttempt(ctx context.Context, tx *store.Tx, identity domain.Identity, to domain.AttemptStatus, now time.Time, terminal []byte, wall time.Time) error {
	if err := validateTransitionIdentity(ctx, tx, identity, to, wall); err != nil {
		return err
	}
	from, err := tx.ReadAttemptStatus(ctx, identity.AttemptID)
	if err != nil {
		return err
	}
	if err := domain.CheckTransition(from, to); err != nil {
		return err
	}
	return persistTransition(ctx, tx, identity, to, now, terminal)
}

// validateTransitionIdentity validates the worker identity. A superseded
// episode still needs to durably acknowledge cancellation of its in-flight
// attempt; only cancellation or abandonment may take that exception, never a
// produced Decision.
func validateTransitionIdentity(ctx context.Context, tx *store.Tx, identity domain.Identity, to domain.AttemptStatus, wall time.Time) error {
	err := ValidateWorkerIdentity(ctx, tx, identity, wall)
	if err == nil {
		return nil
	}
	if !domain.MayAcknowledgeCancellation(to) || !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) {
		return err
	}
	return validateTerminalIdentity(ctx, tx, identity, wall)
}

func persistTransition(ctx context.Context, tx *store.Tx, identity domain.Identity, to domain.AttemptStatus, now time.Time, terminal []byte) error {
	rows, err := writeTransition(ctx, tx, identity, to, now, terminal)
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("attempt %s was not transitioned", identity.AttemptID)
	}
	return nil
}

func writeTransition(ctx context.Context, tx *store.Tx, identity domain.Identity, to domain.AttemptStatus, now time.Time, terminal []byte) (int64, error) {
	switch domain.KindOf(to) {
	case domain.TransitionTerminal:
		return tx.FinishAttempt(ctx, identity, to, store.TimeText(now), terminal)
	case domain.TransitionRunning:
		return tx.StartRunningAttempt(ctx, identity, store.TimeText(now))
	default:
		return tx.SetAttemptStatus(ctx, identity, to)
	}
}
