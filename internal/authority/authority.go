package authority

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

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

// TargetAuthority owns durable target-scoped claims. runtimecontrol.RuntimeOwner remains the
// singleton authority fence; this ledger makes a conflicting target claim
// observable and recoverable after a crash or lease expiry.
type TargetAuthority struct {
	DB           *storage.DB
	Owner        *runtimecontrol.RuntimeOwner
	EpochControl *runtimecontrol.EpochControl
	InstanceID   string
	Lease        time.Duration
	Now          func() time.Time
}

const writeTargetClaimSQL = `
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
			updated_at = excluded.updated_at`

const appendAuthorityEventSQL = `
		INSERT INTO device_authority_events
			(target, device_id, event_type, owner_epoch, owner_instance, boot_id,
			 details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Claim renews or acquires a target claim. An expired claim may be taken over;
// an unexpired claim held by another epoch is durably recorded and rejected.
func (a *TargetAuthority) Claim(ctx context.Context, claim TargetClaim) error {
	if err := a.validateClaim(claim); err != nil {
		return err
	}
	return a.recordClaim(ctx, claim)
}

func (a *TargetAuthority) recordClaim(ctx context.Context, claim TargetClaim) error {
	now := a.now()
	busy := false
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		busy, err = a.claimTx(ctx, tx, claim, now)
		return err
	})
	if err != nil {
		return fmt.Errorf("claim target %q: %w", claim.Target, err)
	}
	if busy {
		return ErrTargetClaimBusy
	}
	return nil
}

// claimTx records a rejected claim and reports busy when another owner holds a
// live claim; otherwise it writes the claim with its fence.
func (a *TargetAuthority) claimTx(ctx context.Context, tx *sql.Tx, claim TargetClaim, now time.Time) (bool, error) {
	if err := a.assertOrdinaryTx(ctx, tx, claim.AuthorityEpoch); err != nil {
		return false, err
	}
	current, err := loadTargetClaim(ctx, tx, claim.Target)
	if err != nil {
		return false, err
	}
	if current.blocks(claim, now) {
		return true, appendAuthorityEventTx(ctx, tx, claim, "claim_rejected", map[string]any{
			"reason": "unexpired_owner", "current_epoch": current.AuthorityEpoch,
			"current_instance": current.OwnerInstance,
		}, now)
	}
	return a.acquireClaimTx(ctx, tx, claim, current, now)
}

func (a *TargetAuthority) acquireClaimTx(ctx context.Context, tx *sql.Tx, claim TargetClaim, current *storedTargetClaim, now time.Time) (bool, error) {
	fence := current.nextFence(claim, now)
	if err := a.writeClaim(ctx, tx, claim, fence, now); err != nil {
		return false, err
	}
	eventType := "claim_acquired"
	if current.activeFor(claim) {
		eventType = "claim_renewed"
	}
	return false, appendAuthorityEventTx(ctx, tx, claim, eventType, map[string]any{"claim_fence": fence}, now)
}

func (a *TargetAuthority) writeClaim(ctx context.Context, tx *sql.Tx, claim TargetClaim, fence int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, writeTargetClaimSQL,
		claim.Target, claim.DeviceID, claim.AuthorityEpoch, claim.OwnerInstance,
		claim.BootID, fence, formatRuntimeTime(now.Add(a.leaseDuration())), formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("write target claim: %w", err)
	}
	return nil
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
	if _, err := tx.ExecContext(ctx, appendAuthorityEventSQL,
		claim.Target, claim.DeviceID, eventType, claim.AuthorityEpoch, claim.OwnerInstance,
		claim.BootID, data, hash[:], formatRuntimeTime(occurredAt)); err != nil {
		return fmt.Errorf("record authority event: %w", err)
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
			return fmt.Errorf("%w", err)
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
