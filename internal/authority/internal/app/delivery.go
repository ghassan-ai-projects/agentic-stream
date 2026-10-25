package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func (s *Service) ClaimForDelivery(ctx context.Context, binding domain.CommandBinding) error {
	claim := binding.Claim()
	if err := s.Claim(ctx, claim); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrClaimRefused, err)
	}
	if err := s.BindCommand(ctx, binding); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrBindingRefused, errors.Join(err, s.releaseOwned(ctx, claim)))
	}
	if err := s.AssertClaim(ctx, claim); err != nil {
		return fmt.Errorf("assert device target authority: %w", err)
	}
	return nil
}

func (s *Service) releaseOwned(ctx context.Context, claim domain.TargetClaim) error {
	if err := s.ReleaseClaim(ctx, claim); err != nil && !errors.Is(err, domain.ErrTargetClaimNotOwned) {
		return err
	}
	return nil
}

func (s *Service) RequireReconciliation(ctx context.Context, opening domain.ReconciliationOpening) error {
	err := s.OpenReconciliation(ctx, opening)
	if err == nil || s.fences.AuthorityLost == nil || !s.fences.AuthorityLost(err) {
		return err
	}
	if recoveryErr := s.OpenReconciliationAfterAuthorityLoss(ctx, opening); recoveryErr != nil {
		return errors.Join(err, recoveryErr)
	}
	return nil
}
