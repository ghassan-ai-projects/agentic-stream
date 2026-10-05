package authority

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// Claim acquires or renews a target claim. While another owner holds a live
// claim, the attempt is audited and refused with ErrTargetClaimBusy.
func (s *Service) Claim(ctx context.Context, claim TargetClaim) error {
	if err := s.checkClaim(claim); err != nil {
		return err
	}
	decision, err := s.recordClaim(ctx, claim, s.now())
	if err != nil {
		return fmt.Errorf("claim target %q: %w", claim.Target, err)
	}
	if decision.Rejected() {
		return ErrTargetClaimBusy
	}
	return nil
}

func (s *Service) recordClaim(ctx context.Context, claim TargetClaim, now time.Time) (domain.ClaimDecision, error) {
	var decision domain.ClaimDecision
	err := s.withAdmittedTx(ctx, claim.Owner.Epoch, func(tx *sql.Tx) error {
		var err error
		decision, err = s.claimTarget(ctx, tx, claim, now)
		return err
	})
	return decision, err
}

// claimTarget decides the claim against the held claim, writes it unless it
// is rejected, and audits the decision either way.
func (s *Service) claimTarget(ctx context.Context, tx *sql.Tx, claim TargetClaim, now time.Time) (domain.ClaimDecision, error) {
	held, err := store.LoadClaim(ctx, tx, claim.Target)
	if err != nil {
		return domain.ClaimDecision{}, err
	}
	decision := domain.DecideClaim(held, claim, now)
	if !decision.Rejected() {
		if err := store.WriteClaim(ctx, tx, claim, decision.Fence, now.Add(s.claimLease), now); err != nil {
			return decision, err
		}
	}
	return decision, store.AppendAuthorityEvent(ctx, tx, domain.ClaimEvent(decision, claim, held, now))
}

// AssertClaim verifies that the exact claim is live and the owner is still
// admitted. Call it immediately before an ordinary transport send: a failure
// after the send is an unknown outcome, because bytes cannot be retracted.
func (s *Service) AssertClaim(ctx context.Context, claim TargetClaim) error {
	if err := s.checkClaim(claim); err != nil {
		return err
	}
	now := s.now()
	err := s.withAdmittedTx(ctx, claim.Owner.Epoch, func(tx *sql.Tx) error {
		held, err := store.LoadClaim(ctx, tx, claim.Target)
		if err != nil {
			return err
		}
		return domain.CheckClaimHeld(held, claim, now)
	})
	if err != nil {
		return fmt.Errorf("assert target %q: %w", claim.Target, err)
	}
	return nil
}

// ReleaseClaim gives up the exact active claim. It is on the priority path, so
// an owner that lost authority can still release; a stale owner cannot release
// a replacement claim.
func (s *Service) ReleaseClaim(ctx context.Context, claim TargetClaim) error {
	if err := s.checkClaim(claim); err != nil {
		return err
	}
	now := s.now()
	err := s.withPriorityTx(ctx, func(tx *sql.Tx) error {
		return releaseClaim(ctx, tx, claim, now)
	})
	if err != nil {
		return fmt.Errorf("release target %q: %w", claim.Target, err)
	}
	return nil
}

func releaseClaim(ctx context.Context, tx *sql.Tx, claim TargetClaim, now time.Time) error {
	held, err := store.LoadClaim(ctx, tx, claim.Target)
	if err != nil {
		return err
	}
	if err := domain.CheckReleasable(held, claim); err != nil {
		return err
	}
	if err := store.MarkClaimReleased(ctx, tx, claim.Target, now); err != nil {
		return err
	}
	return store.AppendAuthorityEvent(ctx, tx, domain.ReleaseEvent(claim, now))
}
