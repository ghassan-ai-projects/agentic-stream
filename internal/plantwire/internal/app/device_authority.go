package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

const unknownOutcomeReason = "plant wire command outcome is unknown"

var ErrAuthorityRequired = errors.New("plantwire: a host that commands needs the device authority and the owner epoch")

func (h *Host) owner() authority.Owner {
	return authority.Owner{Epoch: h.ownerEpoch, Instance: h.authority.OwnerInstance()}
}

func (h *Host) admit(ctx context.Context, t target) (authority.TargetClaim, error) {
	binding, err := h.binding(ctx, t)
	if err != nil {
		return authority.TargetClaim{}, fmt.Errorf("plantwire: command %s not sent: %w", t.order.CommandID, err)
	}
	err = h.authority.ClaimForDelivery(ctx, binding)
	if !errors.Is(err, authority.ErrClaimRefused) && !errors.Is(err, authority.ErrBindingRefused) {
		t.live.holdClaim(binding.Claim())
	}
	if err != nil {
		return authority.TargetClaim{}, fmt.Errorf("plantwire: command %s not sent: %w", t.order.CommandID, err)
	}
	return binding.Claim(), nil
}

func (h *Host) binding(ctx context.Context, t target) (authority.CommandBinding, error) {
	device := authority.DeviceBoot{DeviceID: t.order.Device, BootID: t.order.ExpectedBootID}
	if err := h.checkReconciled(ctx, device, t.view); err != nil {
		return authority.CommandBinding{}, err
	}
	digest, err := domain.CommandDigest(t.order)
	if err != nil {
		return authority.CommandBinding{}, err
	}
	return authority.CommandBinding{CommandID: t.order.CommandID, Target: t.order.Device, Device: device, Owner: h.owner(), CommandDigest: digest}, nil
}

func (h *Host) settleAuthority(ctx context.Context, claim authority.TargetClaim, effect actionport.Effect, err error) (actionport.Effect, error) {
	if actionport.IsUnknownOutcome(err) {
		return effect, h.reconcile(ctx, claim, err)
	}
	if err != nil && !errors.Is(err, ErrCommandFailed) {
		return effect, err
	}
	if assertErr := h.authority.AssertClaim(ctx, claim); assertErr != nil {
		lost := fmt.Errorf("plantwire: authority lost during the command: %w", errors.Join(err, assertErr))
		return effect, h.reconcile(ctx, claim, &actionport.UnknownOutcomeError{Err: lost})
	}
	return effect, err
}

func (h *Host) reconcile(ctx context.Context, claim authority.TargetClaim, unknown error) error {
	persistCtx, cancel := sources.DetachedContext(ctx)
	defer cancel()
	opening := authority.ReconciliationOpening{Device: claim.Device, Owner: claim.Owner, Reason: unknownOutcomeReason}
	if err := h.authority.RequireReconciliation(persistCtx, opening); err != nil {
		return &actionport.UnknownOutcomeError{Err: errors.Join(unknown, fmt.Errorf("plantwire: persist the reconciliation barrier: %w", err))}
	}
	return unknown
}

func (s *session) holdClaim(claim authority.TargetClaim) {
	s.claimsMu.Lock()
	defer s.claimsMu.Unlock()
	s.claims[claim.Target] = claim
}

func (s *session) releaseClaims() {
	s.claimsMu.Lock()
	defer s.claimsMu.Unlock()
	for target, claim := range s.claims {
		if err := s.authority.ReleaseClaim(context.Background(), claim); err != nil && !errors.Is(err, authority.ErrTargetClaimNotOwned) {
			slog.Warn("plant wire target claim not released", "target", target, "error", err)
		}
	}
	clear(s.claims)
}
