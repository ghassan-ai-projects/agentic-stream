package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
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

// storedTargetClaim is the persisted claim on a target.
type storedTargetClaim struct {
	TargetClaim
	fence   int64
	expires time.Time
	status  string
}

// loadTargetClaim returns the current claim on target, or nil when the target
// has never been claimed.
func loadTargetClaim(ctx context.Context, tx *sql.Tx, target string) (*storedTargetClaim, error) {
	current := &storedTargetClaim{TargetClaim: TargetClaim{Target: target}}
	var leaseUntil string
	err := tx.QueryRowContext(ctx, `
		SELECT device_id, owner_epoch, owner_instance, boot_id, claim_fence,
		       lease_until, status
		FROM device_target_claims WHERE target = ?`, target).Scan(
		&current.DeviceID, &current.AuthorityEpoch, &current.OwnerInstance,
		&current.BootID, &current.fence, &leaseUntil, &current.status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load target claim: %w", err)
	}
	current.expires, err = time.Parse(time.RFC3339Nano, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("parse target claim lease: %w", err)
	}
	return current, nil
}

// ownedBy reports whether the stored claim belongs to the claimant's epoch
// and instance.
func (c *storedTargetClaim) ownedBy(claim TargetClaim) bool {
	return c != nil && c.AuthorityEpoch == claim.AuthorityEpoch && c.OwnerInstance == claim.OwnerInstance
}

// live reports whether the stored claim is active and unexpired at now.
func (c *storedTargetClaim) live(now time.Time) bool {
	return c != nil && c.status == "active" && c.expires.After(now)
}

// blocks reports whether another owner holds a live claim.
func (c *storedTargetClaim) blocks(claim TargetClaim, now time.Time) bool {
	return c.live(now) && !c.ownedBy(claim)
}

// activeFor reports whether the claimant already holds an active claim, so a
// write renews rather than acquires it.
func (c *storedTargetClaim) activeFor(claim TargetClaim) bool {
	return c.ownedBy(claim) && c.status == "active"
}

// nextFence keeps the fence for a live renewal by the same owner and
// advances it for every takeover, so a stale owner cannot reuse it.
func (c *storedTargetClaim) nextFence(claim TargetClaim, now time.Time) int64 {
	if c == nil {
		return 1
	}
	if c.ownedBy(claim) && c.live(now) {
		return c.fence
	}
	return c.fence + 1
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

// BindCommand records the device and boot identity for a command immediately
// before transport delivery. A repeated identical binding is idempotent; a
// conflicting binding is rejected before any bytes are sent.
func (a *TargetAuthority) BindCommand(ctx context.Context, binding CommandBinding) error {
	if a == nil || a.DB == nil || !binding.complete() {
		return fmt.Errorf("command binding is not configured")
	}
	if a.InstanceID != "" && a.InstanceID != binding.OwnerInstance {
		return fmt.Errorf("command binding owner instance does not match authority")
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOrdinaryTx(ctx, tx, binding.AuthorityEpoch); err != nil {
			return err
		}
		bound, err := commandAlreadyBound(ctx, tx, binding)
		if err != nil || bound {
			return err
		}
		var digest any
		if binding.CommandDigest != "" {
			decoded, decodeErr := canonicaljson.DecodeDigest(binding.CommandDigest)
			if decodeErr != nil {
				return fmt.Errorf("decode command binding digest: %w", decodeErr)
			}
			digest = decoded
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO device_command_bindings
			(command_id, target, device_id, boot_id, owner_epoch, owner_instance, command_sha256, bound_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, binding.CommandID, binding.Target, binding.DeviceID, binding.BootID,
			binding.AuthorityEpoch, binding.OwnerInstance, digest, formatRuntimeTime(a.now())); err != nil {
			return fmt.Errorf("record command binding: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("bind command %q: %w", binding.CommandID, err)
	}
	return nil
}

func (b CommandBinding) complete() bool {
	return b.CommandID != "" && b.Target != "" && b.DeviceID != "" && b.BootID != "" && b.AuthorityEpoch != "" && b.OwnerInstance != ""
}

// commandAlreadyBound reports whether the command already has this exact
// binding. A binding to a different device lifetime is an error.
func commandAlreadyBound(ctx context.Context, tx *sql.Tx, binding CommandBinding) (bool, error) {
	var stored CommandBinding
	var storedDigest []byte
	err := tx.QueryRowContext(ctx, `SELECT target, device_id, boot_id, owner_epoch, owner_instance, command_sha256
		FROM device_command_bindings WHERE command_id = ?`, binding.CommandID).Scan(&stored.Target, &stored.DeviceID, &stored.BootID, &stored.AuthorityEpoch, &stored.OwnerInstance, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load command binding: %w", err)
	}
	if stored.Target != binding.Target || stored.DeviceID != binding.DeviceID || stored.BootID != binding.BootID ||
		stored.AuthorityEpoch != binding.AuthorityEpoch || stored.OwnerInstance != binding.OwnerInstance ||
		!digestBytesMatch(storedDigest, binding.CommandDigest) {
		return false, fmt.Errorf("command %q is already bound to a different device lifetime", binding.CommandID)
	}
	return true, nil
}

// SafeStopRequested reports whether this device boot has a durable safe-stop
// lifecycle event. There is intentionally no corresponding clear operation;
// a new authority must remain stopped until the external safety owner has
// handled the physical condition.
func (a *TargetAuthority) SafeStopRequested(ctx context.Context, deviceID, bootID string) (bool, error) {
	if a == nil || a.DB == nil || deviceID == "" || bootID == "" {
		return false, fmt.Errorf("device safe-stop state is not configured")
	}
	var count int64
	if err := a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM device_authority_events
		WHERE device_id = ? AND boot_id = ? AND event_type IN ('safe_stop_requested', 'safe_stop_completed', 'safe_stop_failed')`, deviceID, bootID).Scan(&count); err != nil {
		return false, fmt.Errorf("read durable safe-stop state: %w", err)
	}
	return count > 0, nil
}

func digestBytesMatch(stored []byte, reference string) bool {
	if reference == "" {
		return len(stored) == 0
	}
	decoded, err := canonicaljson.DecodeDigest(reference)
	return err == nil && string(stored) == string(decoded)
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

// RecordSafeStop writes a safe-stop lifecycle event without ordinary authority
// admission checks, preserving the priority path during authority failure.
func (a *TargetAuthority) RecordSafeStop(ctx context.Context, claim TargetClaim, eventType string, details map[string]any) error {
	if a == nil || a.DB == nil {
		return fmt.Errorf("target authority is not configured")
	}
	if err := a.validateClaim(claim); err != nil {
		return err
	}
	return recordSafeStopEvent(ctx, a.DB, claim, eventType, details, a.now())
}

func recordSafeStopEvent(ctx context.Context, db *DB, claim TargetClaim, eventType string, details map[string]any, occurredAt time.Time) error {
	if db == nil || claim.Target == "" || claim.DeviceID == "" || claim.BootID == "" || claim.AuthorityEpoch == "" || claim.OwnerInstance == "" {
		return fmt.Errorf("safe-stop target, device, boot, authority, and owner are required")
	}
	switch eventType {
	case "safe_stop_requested", "safe_stop_completed", "safe_stop_failed":
	default:
		return fmt.Errorf("invalid safe-stop event type %q", eventType)
	}
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		return appendAuthorityEventTx(ctx, tx, claim, eventType, details, occurredAt)
	})
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// VerifyStoredJSONDigest verifies a canonical JSON blob and its raw SHA-256.
// It is shared by artifact and safety readers that must fail closed on
// tampered durable evidence.
func VerifyStoredJSONDigest(data, digest []byte) error {
	if len(digest) != sha256.Size {
		return fmt.Errorf("stored digest has %d bytes", len(digest))
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("stored JSON is invalid: %w", err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil || string(canonical) != string(data) {
		return fmt.Errorf("stored JSON is not canonical")
	}
	hash := sha256.Sum256(data)
	if string(hash[:]) != string(digest) {
		return fmt.Errorf("stored JSON digest mismatch")
	}
	return nil
}
