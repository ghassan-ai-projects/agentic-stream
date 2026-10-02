package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Assert verifies an unexpired target claim and the runtime's current epoch.
// Callers should invoke Claim immediately before an ordinary transport send;
// a failed post-send Assert is an unknown outcome, because it cannot retract
// bytes that the gateway may already have accepted.
func (a *TargetAuthority) Assert(ctx context.Context, claim TargetClaim) error {
	if err := a.validateClaim(claim); err != nil {
		return err
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOrdinaryTx(ctx, tx, claim.AuthorityEpoch); err != nil {
			return err
		}
		return a.assertActiveClaimTx(ctx, tx, claim)
	})
	if err != nil {
		return fmt.Errorf("assert target %q: %w", claim.Target, err)
	}
	return nil
}

func (a *TargetAuthority) assertActiveClaimTx(ctx context.Context, tx *sql.Tx, claim TargetClaim) error {
	var status, leaseUntil string
	if err := tx.QueryRowContext(ctx, `
		SELECT status, lease_until FROM device_target_claims
		WHERE target = ? AND device_id = ? AND owner_epoch = ? AND owner_instance = ? AND boot_id = ?`,
		claim.Target, claim.DeviceID, claim.AuthorityEpoch, claim.OwnerInstance, claim.BootID,
	).Scan(&status, &leaseUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTargetClaimNotOwned
		}
		return fmt.Errorf("read target claim: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, leaseUntil)
	if err != nil {
		return fmt.Errorf("parse target claim lease: %w", err)
	}
	if status != "active" || !expires.After(a.now()) {
		return ErrTargetClaimNotOwned
	}
	return nil
}

// AssertRuntime verifies the singleton owner and epoch without requiring a
// target claim. It is used for non-effect state transitions such as recording
// a reconciliation result.
func (a *TargetAuthority) AssertRuntime(ctx context.Context, epoch string) error {
	if a == nil || a.DB == nil || epoch == "" {
		return fmt.Errorf("target authority and epoch are required")
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return a.assertOrdinaryTx(ctx, tx, epoch)
	})
	if err != nil {
		return fmt.Errorf("assert runtime authority: %w", err)
	}
	return nil
}

// Release releases a claim only when its owner identity still matches. A
// stale owner cannot release a replacement claim.
func (a *TargetAuthority) Release(ctx context.Context, claim TargetClaim) error {
	if err := a.validateClaim(claim); err != nil {
		return err
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return a.releaseClaimTx(ctx, tx, claim)
	})
	if err != nil {
		return fmt.Errorf("release target %q: %w", claim.Target, err)
	}
	return nil
}

func (a *TargetAuthority) releaseClaimTx(ctx context.Context, tx *sql.Tx, claim TargetClaim) error {
	if err := markClaimReleased(ctx, tx, claim, a.now()); err != nil {
		return err
	}
	return appendAuthorityEventTx(ctx, tx, claim, "claim_released", nil, a.now())
}

func markClaimReleased(ctx context.Context, tx *sql.Tx, claim TargetClaim, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE device_target_claims SET status = 'released', updated_at = ?
		WHERE target = ? AND device_id = ? AND owner_epoch = ? AND owner_instance = ? AND boot_id = ? AND status = 'active'`,
		formatRuntimeTime(now), claim.Target, claim.DeviceID, claim.AuthorityEpoch, claim.OwnerInstance, claim.BootID)
	if err != nil {
		return fmt.Errorf("release target claim: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count released target claims: %w", err)
	}
	if count == 0 {
		return ErrTargetClaimNotOwned
	}
	return nil
}
