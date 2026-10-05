package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

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
		return a.bindCommandTx(ctx, tx, binding)
	})
	if err != nil {
		return fmt.Errorf("bind command %q: %w", binding.CommandID, err)
	}
	return nil
}

func insertCommandBinding(ctx context.Context, tx *sql.Tx, binding CommandBinding, now time.Time) error {
	var digest any
	if binding.CommandDigest != "" {
		decoded, err := canonicaljson.DecodeDigest(binding.CommandDigest)
		if err != nil {
			return fmt.Errorf("decode command binding digest: %w", err)
		}
		digest = decoded
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO device_command_bindings
		(command_id, target, device_id, boot_id, owner_epoch, owner_instance, command_sha256, bound_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, binding.CommandID, binding.Target, binding.DeviceID, binding.BootID,
		binding.AuthorityEpoch, binding.OwnerInstance, digest, formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("record command binding: %w", err)
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
	err := tx.QueryRowContext(ctx, readCommandBindingSQL, binding.CommandID).Scan(&stored.Target, &stored.DeviceID, &stored.BootID, &stored.AuthorityEpoch, &stored.OwnerInstance, &storedDigest)
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

func recordSafeStopEvent(ctx context.Context, db *storage.DB, claim TargetClaim, eventType string, details map[string]any, occurredAt time.Time) error {
	if db == nil || claim.Target == "" || claim.DeviceID == "" || claim.BootID == "" || claim.AuthorityEpoch == "" || claim.OwnerInstance == "" {
		return fmt.Errorf("safe-stop target, device, boot, authority, and owner are required")
	}
	switch eventType {
	case "safe_stop_requested", "safe_stop_completed", "safe_stop_failed":
	default:
		return fmt.Errorf("invalid safe-stop event type %q", eventType)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return appendAuthorityEventTx(ctx, tx, claim, eventType, details, occurredAt)
	}); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (a *TargetAuthority) bindCommandTx(ctx context.Context, tx *sql.Tx, binding CommandBinding) error {
	if err := a.assertOrdinaryTx(ctx, tx, binding.AuthorityEpoch); err != nil {
		return err
	}
	bound, err := commandAlreadyBound(ctx, tx, binding)
	if err != nil || bound {
		return err
	}
	return insertCommandBinding(ctx, tx, binding, a.now())
}

const readCommandBindingSQL = `SELECT target, device_id, boot_id, owner_epoch, owner_instance, command_sha256
		FROM device_command_bindings WHERE command_id = ?`
