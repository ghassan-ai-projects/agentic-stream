package notify

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

func recordPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64, now time.Time) (bool, error) {
	var skip bool
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		skip, err = recordPoisonAttemptTx(ctx, tx, tenantID, cursor, now)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("persist notification poison attempt: %w", err)
	}
	return skip, nil
}

func recordPoisonAttemptTx(ctx context.Context, tx *sql.Tx, tenantID string, cursor int64, now time.Time) (bool, error) {
	attempts, err := incrementPoisonAttempts(ctx, tx, tenantID, cursor, now)
	if err != nil {
		return false, err
	}
	if attempts < maxNotificationPoisonAttempts {
		return false, nil
	}
	if err := auditTx(ctx, tx, tenantID, "subscriber_skipped", cursor, cursor, now); err != nil {
		return false, fmt.Errorf("audit skipped poison notification: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
		return false, fmt.Errorf("clear notification poison attempts: %w", err)
	}
	return true, nil
}

func incrementPoisonAttempts(ctx context.Context, tx *sql.Tx, tenantID string, cursor int64, now time.Time) (int, error) {
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_poison_attempts (tenant_id, cursor, attempts, last_attempt_at) VALUES (?, ?, 1, ?) ON CONFLICT(tenant_id, cursor) DO UPDATE SET attempts = attempts + 1, last_attempt_at = excluded.last_attempt_at`, tenantID, cursor, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("record notification poison attempt: %w", err)
	}
	var attempts int
	if err := tx.QueryRowContext(ctx, "SELECT attempts FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("read notification poison attempts: %w", err)
	}
	return attempts, nil
}

func clearPoisonAttempt(ctx context.Context, db *storage.DB, tenantID string, cursor int64) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM notification_poison_attempts WHERE tenant_id = ? AND cursor = ?", tenantID, cursor); err != nil {
		return fmt.Errorf("clear notification poison attempt: %w", err)
	}
	return nil
}
