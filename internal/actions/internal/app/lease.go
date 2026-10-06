package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// leaseNext claims the oldest available command for dispatch. It reports found
// only when this dispatcher now holds the lease.
func (s *Service) leaseNext(ctx context.Context) (domain.LeasedCommand, bool, error) {
	var leased domain.LeasedCommand
	found := false
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		leased, found, err = s.leaseIn(ctx, tx)
		return err
	})
	if err != nil {
		return domain.LeasedCommand{}, false, fmt.Errorf("lease command: %w", err)
	}
	return leased, found, nil
}

func (s *Service) leaseIn(ctx context.Context, tx *store.Tx) (domain.LeasedCommand, bool, error) {
	if err := tx.AssertOwner(ctx); err != nil {
		return domain.LeasedCommand{}, false, err
	}
	now := s.clk.Now().UTC()
	candidate, found, err := tx.NextCandidate(ctx, now)
	if err != nil || !found {
		return domain.LeasedCommand{}, false, err
	}
	return s.admit(ctx, tx, candidate.Admit(now), now)
}

func (s *Service) admit(ctx context.Context, tx *store.Tx, admission domain.Admission, now time.Time) (domain.LeasedCommand, bool, error) {
	leased := admission.Leased
	switch admission.Step {
	case domain.AbandonExpiredLease:
		return leased, false, s.abandonExpiredLease(ctx, tx, leased)
	case domain.FailInvalidCommand:
		return leased, false, tx.FailInvalidCommand(ctx, leased.OutboxID, leased.Command.CommandID, admission.FailureCode, now)
	case domain.CloseOutboxOnly:
		return leased, false, tx.CloseOutboxOnly(ctx, leased.OutboxID, admission.OutboxClosure, now)
	default:
		return s.acquireLease(ctx, tx, leased, now)
	}
}

// abandonExpiredLease records an unknown outcome for a command whose earlier
// lease expired mid-dispatch, so no worker can blindly repeat the effect.
func (s *Service) abandonExpiredLease(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand) error {
	s.observeLeaseExpiry()
	return s.finalizeIn(ctx, tx, leased, actionport.Effect{}, &actionport.UnknownOutcomeError{Err: errors.New("lease expired before dispatch")})
}

func (s *Service) observeLeaseExpiry() {
	if s.observer != nil {
		s.observer.ObserveLeaseExpiry()
	}
}

// acquireLease takes a fresh lease on the outbox row and marks the command
// dispatching. It reports false when another dispatcher won the row.
func (s *Service) acquireLease(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand, now time.Time) (domain.LeasedCommand, bool, error) {
	leased.LeaseOwner = s.owner + "/" + s.ids.New(ids.PrefixLease)
	acquired, err := tx.AcquireLease(ctx, leased.OutboxID, leased.LeaseOwner, now.Add(s.leaseFor), now)
	if err != nil || !acquired {
		return leased, false, err
	}
	if err := tx.MarkCommandDispatching(ctx, leased.Command.CommandID, now); err != nil {
		return leased, false, err
	}
	return leased, true, nil
}
