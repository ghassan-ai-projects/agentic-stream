package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ClaimLease writes the singleton lease for the epoch and instance. A lease
// held by another unexpired epoch is left untouched; the caller reads the
// recorded owner back to learn the outcome.
func (t *Tx) ClaimLease(ctx context.Context, epoch, instance, nowText, untilText string) error {
	if _, err := t.q.ExecContext(ctx, claimOwnerLeaseSQL, epoch, instance, nowText, nowText, untilText); err != nil {
		return fmt.Errorf("claim runtime owner: %w", err)
	}
	return nil
}

// RecordedOwner reads the epoch and instance recorded in the lease row.
func (t *Tx) RecordedOwner(ctx context.Context) (epoch, instance string, err error) {
	row := t.q.QueryRowContext(ctx, "SELECT owner_epoch, owner_instance FROM runtime_owner WHERE singleton_id = 1")
	if err := row.Scan(&epoch, &instance); err != nil {
		return "", "", fmt.Errorf("read runtime owner after claim: %w", err)
	}
	return epoch, instance, nil
}

// RenewLease extends the lease of the owning, unexpired epoch and returns the
// rows it changed.
func (t *Tx) RenewLease(ctx context.Context, epoch, instance, nowText, untilText string) (int64, error) {
	result, err := t.q.ExecContext(ctx, renewOwnerLeaseSQL, nowText, untilText, epoch, instance, nowText)
	if err != nil {
		return 0, fmt.Errorf("renew runtime owner: %w", err)
	}
	return affectedOwners(result, "renewed")
}

// ReleaseLease expires the lease of the owning epoch and returns the rows it changed.
func (t *Tx) ReleaseLease(ctx context.Context, epoch, instance, nowText string) (int64, error) {
	result, err := t.q.ExecContext(ctx, releaseOwnerLeaseSQL, nowText, nowText, epoch, instance)
	if err != nil {
		return 0, fmt.Errorf("release runtime owner: %w", err)
	}
	return affectedOwners(result, "released")
}

// HoldsLease reports whether the epoch and instance own an unexpired lease at nowText.
func (t *Tx) HoldsLease(ctx context.Context, epoch, instance, nowText string) (bool, error) {
	_, held, err := storage.QueryOptional[string](ctx, t.q, holdsOwnerLeaseSQL, epoch, instance, nowText)
	if err != nil {
		return false, fmt.Errorf("assert runtime owner: %w", err)
	}
	return held, nil
}

// Recover hands the claiming transaction to the caller's recovery writes.
func (t *Tx) Recover(recover Recovery, now time.Time) error {
	return recover(t.tx, now)
}

func affectedOwners(result sql.Result, verb string) (int64, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count %s runtime owners: %w", verb, err)
	}
	return count, nil
}

const claimOwnerLeaseSQL = `
		INSERT INTO runtime_owner (
			singleton_id, owner_epoch, owner_instance, acquired_at,
			heartbeat_at, lease_until
		) VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(singleton_id) DO UPDATE SET
			owner_epoch = excluded.owner_epoch,
			owner_instance = excluded.owner_instance,
			acquired_at = CASE
				WHEN runtime_owner.owner_epoch = excluded.owner_epoch
					AND runtime_owner.owner_instance = excluded.owner_instance
					THEN runtime_owner.acquired_at
				ELSE excluded.acquired_at
			END,
			heartbeat_at = excluded.heartbeat_at,
			lease_until = excluded.lease_until
		WHERE (runtime_owner.owner_epoch = excluded.owner_epoch
			AND runtime_owner.owner_instance = excluded.owner_instance)
			OR (runtime_owner.owner_epoch <> excluded.owner_epoch
				AND runtime_owner.lease_until <= excluded.heartbeat_at)`

const renewOwnerLeaseSQL = `
		UPDATE runtime_owner
		SET heartbeat_at = ?, lease_until = ?
		WHERE singleton_id = 1 AND owner_epoch = ? AND owner_instance = ? AND lease_until > ?`

const releaseOwnerLeaseSQL = `
		UPDATE runtime_owner SET heartbeat_at = ?, lease_until = ?
		WHERE singleton_id = 1 AND owner_epoch = ? AND owner_instance = ?`

const holdsOwnerLeaseSQL = `
		SELECT owner_epoch FROM runtime_owner
		WHERE singleton_id = 1 AND owner_epoch = ? AND owner_instance = ? AND lease_until > ?`
