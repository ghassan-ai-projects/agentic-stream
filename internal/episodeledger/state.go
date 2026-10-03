package episodeledger

import (
	"context"
	"database/sql"
	"fmt"
)

// Rebind persists the validated live snapshot and consumes one rebind.
func Rebind(ctx context.Context, tx *sql.Tx, episodeID string, version int, digest, request []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET situation_version = ?, snapshot_sha256 = ?, request_json = ?,
 stale_rebind_count = stale_rebind_count + 1 WHERE episode_id = ?`, version, digest, request, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// BindRequest persists the request after a fenced attempt identity is bound.
func BindRequest(ctx context.Context, tx *sql.Tx, episodeID string, request []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET request_json = ? WHERE episode_id = ?`, request, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// AbandonRebind quarantines an invalid live snapshot and consumes one rebind.
func AbandonRebind(ctx context.Context, tx *sql.Tx, episodeID, now string, terminal []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?,
 stale_rebind_count = stale_rebind_count + 1 WHERE episode_id = ?`, now, terminal, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Abandon records a terminal quarantine outcome.
func Abandon(ctx context.Context, tx *sql.Tx, episodeID, now string, terminal []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ? WHERE episode_id = ?`, now, terminal, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// Conclude records the terminal execution outcome.
func Conclude(ctx context.Context, tx *sql.Tx, episodeID, now string, terminal []byte) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?`, now, terminal, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// RetainForRetry retains the episode for the next bounded attempt.
func RetainForRetry(ctx context.Context, tx *sql.Tx, episodeID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL WHERE episode_id = ?`, episodeID); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
