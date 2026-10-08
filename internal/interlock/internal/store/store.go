package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock/internal/domain"
)

// DurableReader reads the singleton interlock inside the caller's transaction.
type DurableReader struct{}

// Assert fails closed when the durable interlock is absent or not ready.
func (DurableReader) Assert(ctx context.Context, tx *sql.Tx, _ string, _ string, _ string) error {
	var status, reason string
	if err := tx.QueryRowContext(ctx, "SELECT status, reason FROM runtime_interlock WHERE singleton_id = 1").Scan(&status, &reason); err != nil {
		return fmt.Errorf("read runtime interlock: %w", err)
	}
	return domain.RequireReady(status, reason) //nolint:wrapcheck // The sentinel is the public contract.
}

// Read returns the singleton interlock inside the caller's transaction.
func Read(ctx context.Context, tx *sql.Tx) (domain.State, error) {
	var state domain.State
	if err := tx.QueryRowContext(ctx, "SELECT status, reason, version, updated_at FROM runtime_interlock WHERE singleton_id = 1").Scan(&state.Status, &state.Reason, &state.Version, &state.UpdatedAt); err != nil {
		return domain.State{}, fmt.Errorf("read runtime interlock: %w", err)
	}
	return state, nil
}

// Change moves the interlock to status, one version after the stored one.
func Change(ctx context.Context, tx *sql.Tx, status, reason, now string) (domain.State, error) {
	current, err := Read(ctx, tx)
	if err != nil {
		return domain.State{}, err
	}
	next, err := domain.NextState(current, status, reason, now)
	if err != nil {
		return domain.State{}, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
	}
	return next, Set(ctx, tx, next.Status, next.Reason, next.Version, next.UpdatedAt)
}

// Set writes a versioned interlock state; a stale version is refused.
func Set(ctx context.Context, tx *sql.Tx, status, reason string, version int64, now string) error {
	if err := domain.ValidateChange(status, reason, version, now); err != nil {
		return err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
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
