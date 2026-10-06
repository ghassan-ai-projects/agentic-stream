package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// Settle releases the reservation and records actual worker cost. If actual
// spend crosses a ceiling, the corresponding kill switch is tripped.
func Settle(ctx context.Context, tx *store.Tx, episodeID string, actual uint64, now string) error {
	if err := domain.CheckSettlement(episodeID, actual, now); err != nil {
		return err
	}
	reservation, err := loadReservation(ctx, tx, episodeID)
	if err != nil {
		return err
	}
	needed, err := reservation.NeedsSettlement(episodeID, actual)
	if err != nil || !needed {
		return err
	}
	return applySettlement(ctx, tx, episodeID, reservation, actual, now)
}

func loadReservation(ctx context.Context, tx *store.Tx, episodeID string) (domain.Reservation, error) {
	reservation, found, err := tx.LoadReservation(ctx, episodeID)
	if err != nil {
		return domain.Reservation{}, err
	}
	if !found {
		return domain.Reservation{}, fmt.Errorf("cost reservation %s is missing", episodeID)
	}
	return reservation, nil
}

// applySettlement records the episode's actual cost and moves its reservation
// to spent in the global and tenant limits.
func applySettlement(ctx context.Context, tx *store.Tx, episodeID string, reservation domain.Reservation, actual uint64, now string) error {
	if err := tx.RecordSettlement(ctx, episodeID, domain.Micro(actual), now); err != nil {
		return err
	}
	if err := settleLimit(ctx, tx, domain.GlobalScope, reservation.Reserved, actual, now); err != nil {
		return err
	}
	return settleLimit(ctx, tx, domain.TenantScope(reservation.TenantID), reservation.Reserved, actual, now)
}

func settleLimit(ctx context.Context, tx *store.Tx, scopeKey string, reserved int64, actual uint64, now string) error {
	exists, err := tx.LimitExists(ctx, scopeKey)
	if err != nil {
		return err
	}
	if !exists {
		return domain.RefuseMissingLimit(scopeKey)
	}
	rows, err := tx.SettleLimit(ctx, scopeKey, reserved, domain.Micro(actual), now)
	if err != nil {
		return err
	}
	return domain.CheckLimitSettled(rows, scopeKey)
}
