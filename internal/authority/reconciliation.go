package authority

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ReconciliationStore persists the boot barrier and the last typed state
// received from a device. A process restart therefore cannot forget an open
// barrier.
type ReconciliationStore struct {
	DB        *storage.DB
	Authority *TargetAuthority
	Now       func() time.Time
}

// stateBinding is a validated device state and the authority binding it.
type stateBinding struct {
	deviceID, bootID, authorityEpoch, ownerInstance string
	stateJSON, stateSHA256                          []byte
}

const updateBoundDeviceStateSQL = `
		UPDATE device_reconciliation SET boot_id = ?, status = ?, opening_boot_id = ?,
			state_json = ?, state_sha256 = ?, authority_epoch = ?,
			last_resolution_status = CASE WHEN boot_id = ? THEN last_resolution_status ELSE NULL END,
			resolution_evidence_json = CASE WHEN boot_id = ? THEN resolution_evidence_json ELSE NULL END,
			resolution_sha256 = CASE WHEN boot_id = ? THEN resolution_sha256 ELSE NULL END,
			resolved_at = CASE WHEN boot_id = ? THEN resolved_at ELSE NULL END,
			updated_at = ? WHERE device_id = ?`

// BindState stores a validated device state and returns whether ordinary
// commands must remain blocked. A changed boot opens the barrier atomically.
func (s *ReconciliationStore) BindState(ctx context.Context, state map[string]any, authorityEpoch, ownerInstance string) (bool, error) {
	if s == nil || s.DB == nil || s.Authority == nil {
		return false, fmt.Errorf("reconciliation store is not configured")
	}
	binding, err := newStateBinding(state, authorityEpoch, ownerInstance)
	if err != nil {
		return false, err
	}
	now := s.now()
	if err := s.Authority.AssertRuntime(ctx, authorityEpoch); err != nil {
		return false, fmt.Errorf("assert authority before binding device state: %w", err)
	}
	return s.recordBoundState(ctx, binding, now)
}

func newStateBinding(state map[string]any, authorityEpoch, ownerInstance string) (stateBinding, error) {
	deviceID, _ := state["device_id"].(string)
	bootID, _ := state["boot_id"].(string)
	if deviceID == "" || bootID == "" || authorityEpoch == "" || ownerInstance == "" {
		return stateBinding{}, fmt.Errorf("device, boot, authority epoch, and owner instance are required")
	}
	stateJSON, err := canonicaljson.Marshal(state)
	if err != nil {
		return stateBinding{}, fmt.Errorf("canonicalize device state: %w", err)
	}
	stateHash := sha256.Sum256(stateJSON)
	return stateBinding{deviceID: deviceID, bootID: bootID, authorityEpoch: authorityEpoch, ownerInstance: ownerInstance,
		stateJSON: stateJSON, stateSHA256: stateHash[:]}, nil
}

func (s *ReconciliationStore) recordBoundState(ctx context.Context, binding stateBinding, now time.Time) (bool, error) {
	required := false
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		required, err = s.bindStateTx(ctx, tx, binding, now)
		return err
	})
	if err != nil {
		return required, fmt.Errorf("bind device state: %w", err)
	}
	return required, nil
}

// bindStateTx records the first state of a device as clear, and otherwise
// keeps the barrier required while it is open or the device rebooted.
func (s *ReconciliationStore) bindStateTx(ctx context.Context, tx *sql.Tx, b stateBinding, now time.Time) (bool, error) {
	if err := s.Authority.assertOrdinaryTx(ctx, tx, b.authorityEpoch); err != nil {
		return false, fmt.Errorf("assert authority while binding device state: %w", err)
	}
	var currentBoot, status string
	err := tx.QueryRowContext(ctx, `SELECT boot_id, status FROM device_reconciliation WHERE device_id = ?`, b.deviceID).Scan(&currentBoot, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, insertFirstState(ctx, tx, b, now)
	}
	if err != nil {
		return false, fmt.Errorf("load reconciliation state: %w", err)
	}
	return updateReconciliationBoot(ctx, tx, b, currentBoot, status, now)
}

func updateReconciliationBoot(ctx context.Context, tx *sql.Tx, b stateBinding, currentBoot, status string, now time.Time) (bool, error) {
	rebooted := currentBoot != b.bootID
	required := status == "required" || rebooted
	openingBoot := currentBoot
	if rebooted {
		openingBoot = b.bootID
		if err := recordReconciliationOpened(ctx, tx, b, currentBoot, now); err != nil {
			return false, err
		}
	}
	return required, updateBoundState(ctx, tx, b, required, openingBoot, now)
}

func insertFirstState(ctx context.Context, tx *sql.Tx, b stateBinding, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO device_reconciliation
			(device_id, boot_id, status, opening_boot_id, state_json, state_sha256,
			 authority_epoch, opened_at, updated_at)
		VALUES (?, ?, 'clear', ?, ?, ?, ?, ?, ?)`,
		b.deviceID, b.bootID, b.bootID, b.stateJSON, b.stateSHA256, b.authorityEpoch, formatRuntimeTime(now), formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("insert reconciliation state: %w", err)
	}
	return nil
}

func recordReconciliationOpened(ctx context.Context, tx *sql.Tx, b stateBinding, previousBoot string, now time.Time) error {
	claim := TargetClaim{Target: b.deviceID, DeviceID: b.deviceID, BootID: b.bootID, AuthorityEpoch: b.authorityEpoch, OwnerInstance: b.ownerInstance}
	if err := appendAuthorityEventTx(ctx, tx, claim, "reconciliation_opened", map[string]any{
		"previous_boot_id": previousBoot, "opening_boot_id": b.bootID,
	}, now); err != nil {
		return fmt.Errorf("record reconciliation opening: %w", err)
	}
	return nil
}

// updateBoundState stores the new state; a new boot clears the previous
// boot's resolution evidence.
func updateBoundState(ctx context.Context, tx *sql.Tx, b stateBinding, required bool, openingBoot string, now time.Time) error {
	newStatus := "clear"
	if required {
		newStatus = "required"
	}
	if _, err := tx.ExecContext(ctx, updateBoundDeviceStateSQL,
		b.bootID, newStatus, openingBoot, b.stateJSON, b.stateSHA256, b.authorityEpoch,
		b.bootID, b.bootID, b.bootID, b.bootID, formatRuntimeTime(now), b.deviceID); err != nil {
		return fmt.Errorf("update reconciliation state: %w", err)
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
