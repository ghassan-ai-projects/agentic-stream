package control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
)

// ErrCostReservationRejected means an aggregate cost limit or kill switch denied
// a new episode reservation. It is an expected admission outcome, not a
// storage failure.
var ErrCostReservationRejected = errors.New("cost ceiling or kill switch rejected")

// CostLedger reserves configured episode cost before admission and settles
// actual worker-reported cost at terminal persistence.
type CostLedger struct{}

// Reserve atomically reserves amount for an episode. A zero max means
// unlimited; a kill switch always rejects new reservations.
func (CostLedger) Reserve(ctx context.Context, tx *sql.Tx, episodeID, tenantID string, amount uint64, now string) error {
	if episodeID == "" || tenantID == "" || now == "" || amount > math.MaxInt64 {
		return fmt.Errorf("invalid cost reservation")
	}
	if amount == 0 {
		if err := requireNoActiveCostControl(ctx, tx, tenantID); err != nil {
			return err
		}
	}
	if err := insertCostReservation(ctx, tx, episodeID, tenantID, amount, now); err != nil {
		return err
	}
	if err := reserveLimit(ctx, tx, "global", amount, now); err != nil {
		return err
	}
	return reserveLimit(ctx, tx, "tenant:"+tenantID, amount, now)
}

// requireNoActiveCostControl rejects an unestimated (zero) reservation while
// a global or tenant ceiling or kill switch is active.
func requireNoActiveCostControl(ctx context.Context, tx *sql.Tx, tenantID string) error {
	rows, err := tx.QueryContext(ctx, "SELECT scope_key, max_micro, kill_switch FROM cost_limits WHERE scope_key IN ('global', ?)", "tenant:"+tenantID)
	if err != nil {
		return fmt.Errorf("read aggregate cost ceilings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := requireInactiveCeiling(rows); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read aggregate cost ceilings: %w", err)
	}
	return nil
}

// requireInactiveCeiling rejects the ceiling at the cursor when it sets a
// limit or engages the kill switch.
func requireInactiveCeiling(rows *sql.Rows) error {
	var scopeKey string
	var maxMicro, killSwitch int64
	if err := rows.Scan(&scopeKey, &maxMicro, &killSwitch); err != nil {
		return fmt.Errorf("scan %s cost ceiling: %w", scopeKey, err)
	}
	if maxMicro > 0 || killSwitch != 0 {
		return fmt.Errorf("%w: cost estimate is required while aggregate cost control is active", ErrCostReservationRejected)
	}
	return nil
}

func insertCostReservation(ctx context.Context, tx *sql.Tx, episodeID, tenantID string, amount uint64, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cost_reservations (reservation_id, episode_id, tenant_id, reserved_micro, status, created_at)
		VALUES (?, ?, ?, ?, 'reserved', ?)`, episodeID, episodeID, tenantID, toInt64(amount), now); err != nil {
		return fmt.Errorf("insert cost reservation: %w", err)
	}
	return nil
}
