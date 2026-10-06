package control

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

// Settle releases the reservation and records actual worker cost. If actual
// spend crosses a ceiling, the corresponding kill switch is tripped.
func (CostLedger) Settle(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error {
	if episodeID == "" || now == "" || actual > math.MaxInt64 {
		return fmt.Errorf("invalid cost settlement")
	}
	reservation, err := loadCostReservation(ctx, tx, episodeID)
	if err != nil {
		return err
	}
	needed, err := reservation.needsSettlement(episodeID, actual)
	if err != nil || !needed {
		return err
	}
	return applySettlement(ctx, tx, episodeID, reservation, actual, now)
}

// applySettlement records the episode's actual cost and moves its reservation
// to spent in the global and tenant limits.
func applySettlement(ctx context.Context, tx *sql.Tx, episodeID string, reservation costReservation, actual uint64, now string) error {
	if err := recordCostSettlement(ctx, tx, episodeID, actual, now); err != nil {
		return err
	}
	if err := settleLimit(ctx, tx, "global", reservation.reserved, actual, now); err != nil {
		return err
	}
	return settleLimit(ctx, tx, "tenant:"+reservation.tenantID, reservation.reserved, actual, now)
}

type costReservation struct {
	tenantID         string
	reserved, actual int64
	status           string
}

func loadCostReservation(ctx context.Context, tx *sql.Tx, episodeID string) (costReservation, error) {
	var reservation costReservation
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, reserved_micro, actual_micro, status FROM cost_reservations WHERE episode_id = ?", episodeID).Scan(&reservation.tenantID, &reservation.reserved, &reservation.actual, &reservation.status); err != nil {
		if err == sql.ErrNoRows {
			return costReservation{}, fmt.Errorf("cost reservation %s is missing", episodeID)
		}
		return costReservation{}, fmt.Errorf("load cost reservation: %w", err)
	}
	return reservation, nil
}

func (r costReservation) needsSettlement(episodeID string, actual uint64) (bool, error) {
	if r.status == "reserved" {
		return true, nil
	}
	if r.status == "settled" && r.actual == toInt64(actual) {
		return false, nil
	}
	return false, fmt.Errorf("cost reservation %s was already settled with a different value", episodeID)
}

func recordCostSettlement(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE cost_reservations SET actual_micro = ?, status = 'settled', settled_at = ?
		WHERE episode_id = ? AND status = 'reserved'`, toInt64(actual), now, episodeID); err != nil {
		return fmt.Errorf("settle cost reservation: %w", err)
	}
	return nil
}
