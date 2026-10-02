package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ErrTargetClaimBusy means another unexpired authority owns the target.
var ErrTargetClaimBusy = errors.New("device target claim is held by another authority")

// ErrTargetClaimNotOwned means a release was attempted by a non-owner.
var ErrTargetClaimNotOwned = errors.New("device target claim is not owned by this authority")

// ErrReconciliationRequired means a device target cannot accept an ordinary
// command until its post-boot state has been reconciled.
var ErrReconciliationRequired = errors.New("device reconciliation is required")

// TargetClaim identifies the authority and device lifetime that may command a
// target. It is checked in SQLite, not only in process memory.
type TargetClaim struct {
	Target         string
	DeviceID       string
	BootID         string
	AuthorityEpoch string
	OwnerInstance  string
}

// CommandBinding ties a dispatched serial command to the device lifetime that
// admitted it. It lets the generic dispatcher validate device evidence without
// making every effector depend on serial-specific fields.
type CommandBinding struct {
	CommandID      string
	Target         string
	DeviceID       string
	BootID         string
	AuthorityEpoch string
	OwnerInstance  string
	CommandDigest  string
}

// TargetAuthority owns durable target-scoped claims. RuntimeOwner remains the
// singleton authority fence; this ledger makes a conflicting target claim
// observable and recoverable after a crash or lease expiry.
type TargetAuthority struct {
	DB           *DB
	Owner        *RuntimeOwner
	EpochControl *EpochControl
	InstanceID   string
	Lease        time.Duration
	Now          func() time.Time
}

// Claim renews or acquires a target claim. An expired claim may be taken over;
// an unexpired claim held by another epoch is durably recorded and rejected.
func (a *TargetAuthority) Claim(ctx context.Context, claim TargetClaim) error {
	if err := a.validateClaim(claim); err != nil {
		return err
	}
	now := a.now()
	var claimError error
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOrdinaryTx(ctx, tx, claim.AuthorityEpoch); err != nil {
			return err
		}
		current, err := loadTargetClaim(ctx, tx, claim.Target)
		if err != nil {
			return err
		}
		if current.blocks(claim, now) {
			if err := appendAuthorityEventTx(ctx, tx, claim, "claim_rejected", map[string]any{
				"reason": "unexpired_owner", "current_epoch": current.AuthorityEpoch,
				"current_instance": current.OwnerInstance,
			}, now); err != nil {
				return err
			}
			claimError = ErrTargetClaimBusy
			return nil
		}
		fence := current.nextFence(claim, now)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO device_target_claims
				(target, device_id, owner_epoch, owner_instance, boot_id, claim_fence, lease_until, status, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?)
			ON CONFLICT(target) DO UPDATE SET
				device_id = excluded.device_id,
				owner_epoch = excluded.owner_epoch,
				owner_instance = excluded.owner_instance,
				boot_id = excluded.boot_id,
				claim_fence = excluded.claim_fence,
				lease_until = excluded.lease_until,
				status = 'active',
				updated_at = excluded.updated_at`,
			claim.Target, claim.DeviceID, claim.AuthorityEpoch, claim.OwnerInstance,
			claim.BootID, fence, formatRuntimeTime(now.Add(a.leaseDuration())), formatRuntimeTime(now)); err != nil {
			return fmt.Errorf("write target claim: %w", err)
		}
		eventType := "claim_acquired"
		if current.activeFor(claim) {
			eventType = "claim_renewed"
		}
		return appendAuthorityEventTx(ctx, tx, claim, eventType, map[string]any{"claim_fence": fence}, now)
	})
	if err != nil {
		return fmt.Errorf("claim target %q: %w", claim.Target, err)
	}
	return claimError
}

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
	})
	if err != nil {
		return fmt.Errorf("assert target %q: %w", claim.Target, err)
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
		result, err := tx.ExecContext(ctx, `
			UPDATE device_target_claims SET status = 'released', updated_at = ?
			WHERE target = ? AND device_id = ? AND owner_epoch = ? AND owner_instance = ? AND boot_id = ? AND status = 'active'`,
			formatRuntimeTime(a.now()), claim.Target, claim.DeviceID, claim.AuthorityEpoch, claim.OwnerInstance, claim.BootID)
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
		return appendAuthorityEventTx(ctx, tx, claim, "claim_released", nil, a.now())
	})
	if err != nil {
		return fmt.Errorf("release target %q: %w", claim.Target, err)
	}
	return nil
}

func (a *TargetAuthority) assertOrdinaryTx(ctx context.Context, tx *sql.Tx, epoch string) error {
	if a.Owner != nil {
		if err := a.Owner.Assert(ctx, tx, epoch); err != nil {
			return fmt.Errorf("runtime authority: %w", err)
		}
	}
	if a.EpochControl != nil {
		if err := a.EpochControl.AssertOrdinaryTx(ctx, tx, epoch); err != nil {
			return err
		}
	}
	return nil
}

func (a *TargetAuthority) validateClaim(claim TargetClaim) error {
	if a == nil || a.DB == nil {
		return fmt.Errorf("target authority is not configured")
	}
	if claim.Target == "" || claim.DeviceID == "" || claim.BootID == "" || claim.AuthorityEpoch == "" || claim.OwnerInstance == "" {
		return fmt.Errorf("target, device, boot, authority epoch, and owner instance are required")
	}
	if a.InstanceID != "" && a.InstanceID != claim.OwnerInstance {
		return fmt.Errorf("claim owner instance %q does not match authority instance %q", claim.OwnerInstance, a.InstanceID)
	}
	if a.Owner != nil && a.Owner.InstanceID != claim.OwnerInstance {
		return fmt.Errorf("claim owner instance %q does not match runtime owner %q", claim.OwnerInstance, a.Owner.InstanceID)
	}
	return nil
}

func (a *TargetAuthority) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a *TargetAuthority) leaseDuration() time.Duration {
	if a.Lease > 0 {
		return a.Lease
	}
	return time.Minute
}

func appendAuthorityEventTx(ctx context.Context, tx *sql.Tx, claim TargetClaim, eventType string, details map[string]any, occurredAt time.Time) error {
	if details == nil {
		details = map[string]any{}
	}
	data, err := canonicaljson.Marshal(details)
	if err != nil {
		return fmt.Errorf("canonicalize authority event: %w", err)
	}
	hash := sha256.Sum256(data)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO device_authority_events
			(target, device_id, event_type, owner_epoch, owner_instance, boot_id,
			 details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		claim.Target, claim.DeviceID, eventType, claim.AuthorityEpoch, claim.OwnerInstance,
		claim.BootID, data, hash[:], formatRuntimeTime(occurredAt)); err != nil {
		return fmt.Errorf("record authority event: %w", err)
	}
	return nil
}
