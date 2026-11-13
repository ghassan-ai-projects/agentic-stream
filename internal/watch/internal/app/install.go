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
	matched, err := s.matchedAtInstall(ctx, watchID, want)
	if err != nil {
		return actionport.Effect{}, err
	}
	now := s.clk.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return installOnce(ctx, tx, watchID, want, matched, now)
	}); err != nil {
		return actionport.Effect{}, fmt.Errorf("watch condition transaction: %w", err)
	}
	return actionport.Effect{ProviderResult: domain.InstallResult(watchID)}, nil
}

func (s *Service) matchedAtInstall(ctx context.Context, watchID string, want domain.Condition) (*bool, error) {
	if s.features == nil {
		return nil, nil
	}
	features, found, err := s.features(ctx, want.SituationID)
	if err != nil {
		return nil, fmt.Errorf("features of situation %s at the install of watch %s: %w", want.SituationID, watchID, err)
	}
	if !found {
		return nil, nil
	}
	matched := matchesFeatures(ctx, watchID, "install", domain.ActiveWatch{Expression: want.Expression, SituationID: want.SituationID, Target: want.Target}, features)
	return &matched, nil
}

func installOnce(ctx context.Context, tx *store.Tx, watchID string, want domain.Condition, matched *bool, now time.Time) error {
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
	if err := tx.InsertCondition(ctx, watchID, want, matched, now); err != nil {
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

func assertGuards(ctx context.Context, tx *store.Tx, tenantID, target string) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	return tx.AssertInterlock(ctx)
}
