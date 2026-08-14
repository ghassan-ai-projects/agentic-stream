// Package costcontrol provides durable aggregate cognition cost controls.
package costcontrol

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
)

// ErrReservationRejected means an aggregate cost limit or kill switch denied
// a new episode reservation. It is an expected admission outcome, not a
// storage failure.
var ErrReservationRejected = errors.New("cost ceiling or kill switch rejected")

// Controller reserves configured episode cost before admission and settles
// actual worker-reported cost at terminal persistence.
type Controller struct{}

// Reserve atomically reserves amount for an episode. A zero max means
// unlimited; a kill switch always rejects new reservations.
func (Controller) Reserve(ctx context.Context, tx *sql.Tx, episodeID, tenantID string, amount uint64, now string) error {
	if episodeID == "" || tenantID == "" || now == "" || amount > math.MaxInt64 {
		return fmt.Errorf("invalid cost reservation")
	}
	if amount == 0 {
		rows, err := tx.QueryContext(ctx, "SELECT scope_key, max_micro, kill_switch FROM cost_limits WHERE scope_key IN ('global', ?)", "tenant:"+tenantID)
		if err != nil {
			return fmt.Errorf("read aggregate cost ceilings: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var scopeKey string
			var maxMicro, killSwitch int64
			if err := rows.Scan(&scopeKey, &maxMicro, &killSwitch); err != nil {
				return fmt.Errorf("scan %s cost ceiling: %w", scopeKey, err)
			}
			if maxMicro > 0 || killSwitch != 0 {
				return fmt.Errorf("%w: cost estimate is required while aggregate cost control is active", ErrReservationRejected)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read aggregate cost ceilings: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cost_reservations (reservation_id, episode_id, tenant_id, reserved_micro, status, created_at)
		VALUES (?, ?, ?, ?, 'reserved', ?)`, episodeID, episodeID, tenantID, int64(amount), now); err != nil {
		return fmt.Errorf("insert cost reservation: %w", err)
	}
	if err := reserveLimit(ctx, tx, "global", "", amount, now); err != nil {
		return err
	}
	if err := reserveLimit(ctx, tx, "tenant:"+tenantID, tenantID, amount, now); err != nil {
		return err
	}
	return nil
}

func reserveLimit(ctx context.Context, tx *sql.Tx, scopeKey, tenantID string, amount uint64, now string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&exists); err == sql.ErrNoRows {
		if scopeKey == "global" {
			return fmt.Errorf("global cost limit is missing")
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("read %s cost limit: %w", scopeKey, err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cost_limits
		SET reserved_micro = reserved_micro + ?, updated_at = ?
		WHERE scope_key = ? AND kill_switch = 0
		  AND (max_micro = 0 OR max_micro >= spent_micro + reserved_micro + ?)`, toInt64(amount), now, scopeKey, toInt64(amount))
	if err != nil {
		return fmt.Errorf("reserve %s cost: %w", scopeKey, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve %s cost rows affected: %w", scopeKey, err)
	}
	if count != 1 {
		return fmt.Errorf("%w: %s", ErrReservationRejected, scopeKey)
	}
	return nil
}

// Settle releases the reservation and records actual worker cost. If actual
// spend crosses a ceiling, the corresponding kill switch is tripped.
func (Controller) Settle(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error {
	if episodeID == "" || now == "" || actual > math.MaxInt64 {
		return fmt.Errorf("invalid cost settlement")
	}
	var tenantID string
	var reserved int64
	var recordedActual int64
	var status string
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, reserved_micro, actual_micro, status FROM cost_reservations WHERE episode_id = ?", episodeID).Scan(&tenantID, &reserved, &recordedActual, &status); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("cost reservation %s is missing", episodeID)
		}
		return fmt.Errorf("load cost reservation: %w", err)
	}
	if status != "reserved" {
		if status == "settled" && recordedActual == toInt64(actual) {
			return nil
		}
		return fmt.Errorf("cost reservation %s was already settled with a different value", episodeID)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cost_reservations SET actual_micro = ?, status = 'settled', settled_at = ?
		WHERE episode_id = ? AND status = 'reserved'`, int64(actual), now, episodeID); err != nil {
		return fmt.Errorf("settle cost reservation: %w", err)
	}
	if err := settleLimit(ctx, tx, "global", reserved, actual, now); err != nil {
		return err
	}
	if err := settleLimit(ctx, tx, "tenant:"+tenantID, reserved, actual, now); err != nil {
		return err
	}
	return nil
}

func settleLimit(ctx context.Context, tx *sql.Tx, scopeKey string, reserved int64, actual uint64, now string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&exists); err == sql.ErrNoRows {
		if scopeKey == "global" {
			return fmt.Errorf("global cost limit is missing")
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("read %s cost limit: %w", scopeKey, err)
	}
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

// SetLimit configures a ceiling or kill switch. It is intended to run inside
// an owner-fenced transaction.
func SetLimit(ctx context.Context, tx *sql.Tx, scopeKey, tenantID string, maxMicro uint64, killSwitch bool, now string) error {
	if scopeKey == "" || now == "" || maxMicro > math.MaxInt64 {
		return fmt.Errorf("invalid cost limit")
	}
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
