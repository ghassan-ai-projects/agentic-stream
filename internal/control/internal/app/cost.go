package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// Reserve atomically reserves amount for an episode in the caller's
// transaction. A zero max means unlimited; a kill switch always rejects new
// reservations.
func Reserve(ctx context.Context, tx *store.Tx, episodeID, tenantID string, amount uint64, now string) error {
	if err := domain.CheckReservation(episodeID, tenantID, amount, now); err != nil {
		return err
	}
	if amount == 0 {
		if err := requireNoActiveCostControl(ctx, tx, tenantID); err != nil {
			return err
		}
	}
	if err := tx.InsertReservation(ctx, episodeID, tenantID, domain.Micro(amount), now); err != nil {
		return err
	}
	if err := reserveLimit(ctx, tx, domain.GlobalScope, amount, now); err != nil {
		return err
	}
	return reserveLimit(ctx, tx, domain.TenantScope(tenantID), amount, now)
}

// requireNoActiveCostControl rejects an unestimated (zero) reservation while a
// global or tenant ceiling or kill switch is active.
func requireNoActiveCostControl(ctx context.Context, tx *store.Tx, tenantID string) error {
	ceilings, err := tx.CeilingsOf(ctx, domain.GlobalScope, domain.TenantScope(tenantID))
	if err != nil {
		return err
	}
	for _, ceiling := range ceilings {
		if err := domain.CheckNoActiveCeiling(ceiling.MaxMicro, ceiling.KillSwitch); err != nil {
			return err
		}
	}
	return nil
}

func reserveLimit(ctx context.Context, tx *store.Tx, scopeKey string, amount uint64, now string) error {
	exists, err := tx.LimitExists(ctx, scopeKey)
	if err != nil {
		return err
	}
	if !exists {
		return domain.RefuseMissingLimit(scopeKey)
	}
	rows, err := tx.ReserveAvailable(ctx, scopeKey, domain.Micro(amount), now)
	if err != nil {
		return err
	}
	return domain.CheckReserved(rows, scopeKey)
}
