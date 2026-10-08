package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// LimitExists reports whether the scope has a cost limit row.
func (t *Tx) LimitExists(ctx context.Context, scopeKey string) (bool, error) {
	var exists int
	err := t.q.QueryRowContext(ctx, "SELECT 1 FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s cost limit: %w", scopeKey, err)
	}
	return true, nil
}

// ReadLimit reads the scope's ceiling and kill switch.
func (t *Tx) ReadLimit(ctx context.Context, scopeKey string) (maxMicro int64, killSwitch bool, err error) {
	var kill int
	if err := t.q.QueryRowContext(ctx, "SELECT max_micro, kill_switch FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&maxMicro, &kill); err != nil {
		return 0, false, fmt.Errorf("load %s: %w", scopeKey, err)
	}
	return maxMicro, kill != 0, nil
}

// WriteLimit upserts a ceiling and kill switch and returns the rows it changed
// (zero when the row count could not be read).
func (t *Tx) WriteLimit(ctx context.Context, scopeKey, tenantID string, maxMicro int64, killSwitch bool, now string) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		INSERT INTO cost_limits (scope_key, tenant_id, max_micro, kill_switch, updated_at)
		VALUES (?, NULLIF(?, ''), ?, ?, ?)
		ON CONFLICT(scope_key) DO UPDATE SET tenant_id = excluded.tenant_id,
			max_micro = excluded.max_micro, kill_switch = excluded.kill_switch, updated_at = excluded.updated_at`,
		scopeKey, tenantID, maxMicro, storage.BoolInt(killSwitch), now)
	if err != nil {
		return 0, fmt.Errorf("set cost limit: %w", err)
	}
	return storage.RowsAffected(result), nil
}

// ReserveAvailable adds the amount to the scope's reserved cost when the kill
// switch is off and the ceiling has room, and returns the rows it changed.
func (t *Tx) ReserveAvailable(ctx context.Context, scopeKey string, amount int64, now string) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		UPDATE cost_limits
		SET reserved_micro = reserved_micro + ?, updated_at = ?
		WHERE scope_key = ? AND kill_switch = 0
		  AND (max_micro = 0 OR max_micro >= spent_micro + reserved_micro + ?)`, amount, now, scopeKey, amount)
	if err != nil {
		return 0, fmt.Errorf("reserve %s cost: %w", scopeKey, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reserve %s cost rows affected: %w", scopeKey, err)
	}
	return count, nil
}

// SettleLimit moves the reservation to spent in the scope, trips the kill
// switch when spend reaches the ceiling, and returns the rows it changed (zero
// when the row count could not be read).
func (t *Tx) SettleLimit(ctx context.Context, scopeKey string, reserved, actual int64, now string) (int64, error) {
	result, err := t.q.ExecContext(ctx, `
		UPDATE cost_limits
		SET reserved_micro = MAX(0, reserved_micro - ?),
		    spent_micro = spent_micro + ?,
		    kill_switch = CASE WHEN max_micro > 0 AND spent_micro + ? >= max_micro THEN 1 ELSE kill_switch END,
		    updated_at = ?
		WHERE scope_key = ?`, reserved, actual, actual, now, scopeKey)
	if err != nil {
		return 0, fmt.Errorf("settle %s cost: %w", scopeKey, err)
	}
	return storage.RowsAffected(result), nil
}
