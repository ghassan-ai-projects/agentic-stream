package control

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

// SetCostLimit configures a ceiling or kill switch. It is intended to run inside
// an owner-fenced transaction.
func SetCostLimit(ctx context.Context, tx *sql.Tx, scopeKey, tenantID string, maxMicro uint64, killSwitch bool, now string) error {
	if scopeKey == "" || now == "" || maxMicro > math.MaxInt64 {
		return fmt.Errorf("invalid cost limit")
	}
	return writeCostLimit(ctx, tx, scopeKey, tenantID, maxMicro, killSwitch, now)
}

func writeCostLimit(ctx context.Context, tx *sql.Tx, scopeKey, tenantID string, maxMicro uint64, killSwitch bool, now string) error {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO cost_limits (scope_key, tenant_id, max_micro, kill_switch, updated_at)
		VALUES (?, NULLIF(?, ''), ?, ?, ?)
		ON CONFLICT(scope_key) DO UPDATE SET tenant_id = excluded.tenant_id,
			max_micro = excluded.max_micro, kill_switch = excluded.kill_switch, updated_at = excluded.updated_at`,
		scopeKey, tenantID, toInt64(maxMicro), boolInt(killSwitch), now)
	if err != nil {
		return fmt.Errorf("set cost limit: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("cost limit was not updated")
	}
	return nil
}

func reserveLimit(ctx context.Context, tx *sql.Tx, scopeKey string, amount uint64, now string) error {
	exists, err := costLimitExists(ctx, tx, scopeKey)
	if err != nil || !exists {
		return err
	}
	return reserveAvailableCost(ctx, tx, scopeKey, amount, now)
}

func costLimitExists(ctx context.Context, tx *sql.Tx, scopeKey string) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&exists); err == sql.ErrNoRows {
		if scopeKey == "global" {
			return false, fmt.Errorf("global cost limit is missing")
		}
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read %s cost limit: %w", scopeKey, err)
	}
	return true, nil
}

func reserveAvailableCost(ctx context.Context, tx *sql.Tx, scopeKey string, amount uint64, now string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE cost_limits
		SET reserved_micro = reserved_micro + ?, updated_at = ?
		WHERE scope_key = ? AND kill_switch = 0
		  AND (max_micro = 0 OR max_micro >= spent_micro + reserved_micro + ?)`, toInt64(amount), now, scopeKey, toInt64(amount))
	if err != nil {
		return fmt.Errorf("reserve %s cost: %w", scopeKey, err)
	}
	return requireReserved(result, scopeKey)
}

// requireReserved rejects a reservation that no limit row admitted.
func requireReserved(result sql.Result, scopeKey string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve %s cost rows affected: %w", scopeKey, err)
	}
	if count != 1 {
		return fmt.Errorf("%w: %s", ErrCostReservationRejected, scopeKey)
	}
	return nil
}

func settleLimit(ctx context.Context, tx *sql.Tx, scopeKey string, reserved int64, actual uint64, now string) error {
	exists, err := costLimitExists(ctx, tx, scopeKey)
	if err != nil || !exists {
		return err
	}
	return recordLimitSettlement(ctx, tx, scopeKey, reserved, actual, now)
}

func recordLimitSettlement(ctx context.Context, tx *sql.Tx, scopeKey string, reserved int64, actual uint64, now string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE cost_limits
		SET reserved_micro = MAX(0, reserved_micro - ?),
		    spent_micro = spent_micro + ?,
		    kill_switch = CASE WHEN max_micro > 0 AND spent_micro + ? >= max_micro THEN 1 ELSE kill_switch END,
		    updated_at = ?
		WHERE scope_key = ?`, reserved, toInt64(actual), toInt64(actual), now, scopeKey)
	if err != nil {
		return fmt.Errorf("settle %s cost: %w", scopeKey, err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("cost limit %s is missing", scopeKey)
	}
	return nil
}

// toInt64 is called only after the public methods enforce SQLite's signed
// integer range.
//
//nolint:gosec // range validation is performed by the caller.
func toInt64(value uint64) int64 {
	return int64(value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
