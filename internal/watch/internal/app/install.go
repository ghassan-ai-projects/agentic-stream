package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// Dispatch installs one watch condition and is idempotent by watch identity.
func (s *Service) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	if err := domain.RequireRoute(command); err != nil {
		return actionport.Effect{}, err
	}
	want, err := domain.ConditionFromCommand(command, s.clk.Now().UTC())
	if err != nil {
		return actionport.Effect{}, err
	}
	return s.install(ctx, command, want)
}

// DispatchAuthorized runs the final authorization check before installing.
func (s *Service) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, errors.New("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("check dispatch authorization: %w", err)
	}
	return s.Dispatch(ctx, command)
}

func (s *Service) install(ctx context.Context, command actionport.Command, want domain.Condition) (actionport.Effect, error) {
	watchID := domain.WatchID(command)
	now := s.clk.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return installOnce(ctx, tx, watchID, want, now)
	}); err != nil {
		return actionport.Effect{}, fmt.Errorf("watch condition transaction: %w", err)
	}
	return actionport.Effect{ProviderResult: domain.InstallResult(watchID)}, nil
}

// installOnce installs the watch, or accepts an identical earlier install of
// the same watch ID and rejects a conflicting one.
func installOnce(ctx context.Context, tx *store.Tx, watchID string, want domain.Condition, now time.Time) error {
	existing, found, err := tx.LoadCondition(ctx, watchID)
	if err != nil {
		return fmt.Errorf("load existing watch condition: %w", err)
	}
	if found {
		return want.SameAs(existing)
	}
	if err := assertGuards(ctx, tx, want.TenantID, want.Target); err != nil {
		return err
	}
	if err := tx.InsertCondition(ctx, watchID, want, now); err != nil {
		return err
	}
	return verifyInstalled(ctx, tx, watchID, want)
}

func verifyInstalled(ctx context.Context, tx *store.Tx, watchID string, want domain.Condition) error {
	stored, found, err := tx.LoadCondition(ctx, watchID)
	if err != nil {
		return fmt.Errorf("verify installed watch condition: %w", err)
	}
	if !found {
		return fmt.Errorf("verify installed watch condition: %w", store.ErrConditionMissing)
	}
	return want.SameAs(stored)
}

// assertGuards re-checks runtime ownership, then the governance interlock.
func assertGuards(ctx context.Context, tx *store.Tx, tenantID, target string) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	return tx.AssertInterlock(ctx)
}
