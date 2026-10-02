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

// ReconciliationStore persists the boot barrier and the last typed state
// received from a device. A process restart therefore cannot forget an open
// barrier.
type ReconciliationStore struct {
	DB        *DB
	Authority *TargetAuthority
	Now       func() time.Time
}

// BindState stores a validated device state and returns whether ordinary
// commands must remain blocked. A changed boot opens the barrier atomically.
func (s *ReconciliationStore) BindState(ctx context.Context, state map[string]any, authorityEpoch, ownerInstance string) (bool, error) {
	if s == nil || s.DB == nil || s.Authority == nil {
		return false, fmt.Errorf("reconciliation store is not configured")
	}
	deviceID, _ := state["device_id"].(string)
	bootID, _ := state["boot_id"].(string)
	if deviceID == "" || bootID == "" || authorityEpoch == "" || ownerInstance == "" {
		return false, fmt.Errorf("device, boot, authority epoch, and owner instance are required")
	}
	stateJSON, err := canonicaljson.Marshal(state)
	if err != nil {
		return false, fmt.Errorf("canonicalize device state: %w", err)
	}
	stateHash := sha256.Sum256(stateJSON)
	now := s.now()
	if err := s.Authority.AssertRuntime(ctx, authorityEpoch); err != nil {
		return false, fmt.Errorf("assert authority before binding device state: %w", err)
	}
	required := false
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.Authority.assertOrdinaryTx(ctx, tx, authorityEpoch); err != nil {
			return fmt.Errorf("assert authority while binding device state: %w", err)
		}
		var currentBoot, status string
		err := tx.QueryRowContext(ctx, `SELECT boot_id, status FROM device_reconciliation WHERE device_id = ?`, deviceID).Scan(&currentBoot, &status)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO device_reconciliation
					(device_id, boot_id, status, opening_boot_id, state_json, state_sha256,
					 authority_epoch, opened_at, updated_at)
				VALUES (?, ?, 'clear', ?, ?, ?, ?, ?, ?)`,
				deviceID, bootID, bootID, stateJSON, stateHash[:], authorityEpoch, formatRuntimeTime(now), formatRuntimeTime(now))
			if err != nil {
				return fmt.Errorf("insert reconciliation state: %w", err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("load reconciliation state: %w", err)
		}
		required = status == "required" || currentBoot != bootID
		openingBoot := currentBoot
		if currentBoot != bootID {
			openingBoot = bootID
			if err := appendAuthorityEventTx(ctx, tx, TargetClaim{Target: deviceID, DeviceID: deviceID, BootID: bootID, AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance}, "reconciliation_opened", map[string]any{
				"previous_boot_id": currentBoot, "opening_boot_id": bootID,
			}, now); err != nil {
				return fmt.Errorf("record reconciliation opening: %w", err)
			}
		}
		newStatus := "clear"
		if required {
			newStatus = "required"
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE device_reconciliation SET boot_id = ?, status = ?, opening_boot_id = ?,
				state_json = ?, state_sha256 = ?, authority_epoch = ?,
				last_resolution_status = CASE WHEN boot_id = ? THEN last_resolution_status ELSE NULL END,
				resolution_evidence_json = CASE WHEN boot_id = ? THEN resolution_evidence_json ELSE NULL END,
				resolution_sha256 = CASE WHEN boot_id = ? THEN resolution_sha256 ELSE NULL END,
				resolved_at = CASE WHEN boot_id = ? THEN resolved_at ELSE NULL END,
				updated_at = ? WHERE device_id = ?`,
			bootID, newStatus, openingBoot, stateJSON, stateHash[:], authorityEpoch,
			bootID, bootID, bootID, bootID, formatRuntimeTime(now), deviceID)
		if err != nil {
			return fmt.Errorf("update reconciliation state: %w", err)
		}
		return nil
	})
	if err != nil {
		return required, fmt.Errorf("bind device state: %w", err)
	}
	return required, nil
}

// Require opens the durable reconciliation barrier for the current device
// boot. It is used when a command may have crossed the gateway but its receipt
// cannot be trusted; a later process must observe the same required state.
func (s *ReconciliationStore) Require(ctx context.Context, deviceID, bootID, authorityEpoch, ownerInstance, reason string) error {
	if err := s.validateRequirement(deviceID, bootID, authorityEpoch, ownerInstance, reason); err != nil {
		return err
	}
	return s.requireBarrier(ctx, deviceID, bootID, authorityEpoch, ownerInstance, reason, true)
}

// RequireAfterAuthorityLoss opens the same boot-bound safety barrier without
// ordinary runtime admission. It exists for an unknown outcome whose transport
// bytes may already have crossed the gateway when the owner lease expires or
// the epoch is fenced. It only makes future commands safer; resolution still
// requires a new owner and independent evidence.
func (s *ReconciliationStore) RequireAfterAuthorityLoss(ctx context.Context, deviceID, bootID, authorityEpoch, ownerInstance, reason string) error {
	if err := s.validateRequirement(deviceID, bootID, authorityEpoch, ownerInstance, reason); err != nil {
		return err
	}
	return s.requireBarrier(ctx, deviceID, bootID, authorityEpoch, ownerInstance, reason, false)
}

func (s *ReconciliationStore) validateRequirement(deviceID, bootID, authorityEpoch, ownerInstance, reason string) error {
	if s == nil || s.DB == nil || s.Authority == nil || deviceID == "" || bootID == "" || authorityEpoch == "" || ownerInstance == "" || reason == "" {
		return fmt.Errorf("device, boot, authority, and reconciliation reason are required")
	}
	return nil
}

func (s *ReconciliationStore) requireBarrier(ctx context.Context, deviceID, bootID, authorityEpoch, ownerInstance, reason string, assertAuthority bool) error {
	now := s.now()
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if assertAuthority {
			if err := s.Authority.assertOrdinaryTx(ctx, tx, authorityEpoch); err != nil {
				return fmt.Errorf("assert authority while opening reconciliation: %w", err)
			}
		}
		var currentBoot, status string
		if err := tx.QueryRowContext(ctx, `SELECT boot_id, status FROM device_reconciliation WHERE device_id = ?`, deviceID).Scan(&currentBoot, &status); err != nil {
			return fmt.Errorf("load reconciliation barrier: %w", err)
		}
		if currentBoot != bootID {
			return fmt.Errorf("reconciliation boot %q does not match current device boot %q", bootID, currentBoot)
		}
		if status != "required" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE device_reconciliation SET status = 'required',
					last_resolution_status = NULL, resolution_evidence_json = NULL,
					resolution_sha256 = NULL, resolved_at = NULL, updated_at = ?
				WHERE device_id = ? AND boot_id = ?`,
				formatRuntimeTime(now), deviceID, bootID); err != nil {
				return fmt.Errorf("open reconciliation barrier: %w", err)
			}
		}
		return appendAuthorityEventTx(ctx, tx, TargetClaim{
			Target: deviceID, DeviceID: deviceID, BootID: bootID,
			AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance,
		}, "reconciliation_opened", map[string]any{"reason": reason}, now)
	})
	if err != nil {
		return fmt.Errorf("require device reconciliation: %w", err)
	}
	return nil
}

// Required reports the durable barrier state for a device.
func (s *ReconciliationStore) Required(ctx context.Context, deviceID string) (bool, error) {
	if s == nil || s.DB == nil || deviceID == "" {
		return false, fmt.Errorf("reconciliation store is not configured")
	}
	var status string
	if err := s.DB.QueryRowContext(ctx, `SELECT status FROM device_reconciliation WHERE device_id = ?`, deviceID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read reconciliation barrier: %w", err)
	}
	return status == "required", nil
}

// RecordSafeStop durably records a safe-stop lifecycle event. It deliberately
// does not assert ordinary authority: the safe-stop path must remain available
// while ordinary authority is draining, expired, or fenced.
func (s *ReconciliationStore) RecordSafeStop(ctx context.Context, target, deviceID, bootID, authorityEpoch, ownerInstance, eventType string, details map[string]any) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("reconciliation store is not configured")
	}
	return recordSafeStopEvent(ctx, s.DB, TargetClaim{
		Target: target, DeviceID: deviceID, BootID: bootID,
		AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance,
	}, eventType, details, s.now())
}

func (s *ReconciliationStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
