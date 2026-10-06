package store

import (
	"context"
	"fmt"
	"time"
)

// CountPoisonAttempt records one more delivery attempt of a poison
// notification and returns the attempts so far.
func (tx *Tx) CountPoisonAttempt(ctx context.Context, tenantID string, cursor int64, at time.Time) (int, error) {
	_, err := tx.q.ExecContext(ctx, `INSERT INTO notification_poison_attempts (tenant_id, cursor, attempts, last_attempt_at) VALUES (?, ?, 1, ?) ON CONFLICT(tenant_id, cursor) DO UPDATE SET attempts = attempts + 1, last_attempt_at = excluded.last_attempt_at`, tenantID, cursor, formatTime(at))
	if err != nil {
		return 0, fmt.Errorf("record notification poison attempt: %w", err)
	}
	var attempts int
	if err := tx.q.QueryRowContext(ctx, "SELECT attempts FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("read notification poison attempts: %w", err)
	}
	return attempts, nil
}

// ClearPoisonAttempts forgets the attempts recorded for a cursor.
func (tx *Tx) ClearPoisonAttempts(ctx context.Context, tenantID string, cursor int64) error {
	if _, err := tx.q.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
		return fmt.Errorf("clear notification poison attempts: %w", err)
	}
	return nil
}
