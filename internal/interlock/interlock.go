// Package interlock provides the read-only action readiness boundary.
package interlock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrTripped means the action plane is globally blocked by a durable interlock.
var ErrTripped = errors.New("runtime interlock is tripped")

// Reader is the narrow, read-only surface used immediately before command
// creation and again immediately before effect delivery.
type Reader interface {
	Assert(context.Context, *sql.Tx, string, string, string) error
}

// DurableReader reads the singleton interlock inside the caller's transaction.
type DurableReader struct{}

// Assert fails closed when the durable interlock is absent or not ready.
func (DurableReader) Assert(ctx context.Context, tx *sql.Tx, _ string, _ string, _ string) error {
	var status, reason string
	if err := tx.QueryRowContext(ctx, "SELECT status, reason FROM runtime_interlock WHERE singleton_id = 1").Scan(&status, &reason); err != nil {
		return fmt.Errorf("read runtime interlock: %w", err)
	}
	if status != "ready" {
		return fmt.Errorf("%w: %s", ErrTripped, reason)
	}
	return nil
}

// Set changes the durable interlock state. Callers must separately fence this
// mutation with the active runtime owner.
func Set(ctx context.Context, tx *sql.Tx, status, reason string, version int64, now string) error {
	if status != "ready" && status != "tripped" {
		return fmt.Errorf("invalid interlock status %q", status)
	}
	if reason == "" || version < 1 || now == "" {
		return fmt.Errorf("interlock reason, version, and timestamp are required")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE runtime_interlock SET status = ?, reason = ?, version = ?, updated_at = ?
		WHERE singleton_id = 1 AND version < ?`, status, reason, version, now, version)
	if err != nil {
		return fmt.Errorf("set runtime interlock: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("runtime interlock row was not updated")
	}
	return nil
}
