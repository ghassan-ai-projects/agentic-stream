package authority

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

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
	claim := TargetClaim{Target: deviceID, DeviceID: deviceID, BootID: bootID, AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance}
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return s.requireBarrierTx(ctx, tx, claim, reason, now, assertAuthority)
	})
	if err != nil {
		return fmt.Errorf("require device reconciliation: %w", err)
	}
	return nil
}

func (s *ReconciliationStore) requireBarrierTx(ctx context.Context, tx *sql.Tx, claim TargetClaim, reason string, now time.Time, assertAuthority bool) error {
	if assertAuthority {
		if err := s.Authority.assertOrdinaryTx(ctx, tx, claim.AuthorityEpoch); err != nil {
			return fmt.Errorf("assert authority while opening reconciliation: %w", err)
		}
	}
	if err := openBarrierForBoot(ctx, tx, claim.DeviceID, claim.BootID, now); err != nil {
		return err
	}
	return appendAuthorityEventTx(ctx, tx, claim, "reconciliation_opened", map[string]any{"reason": reason}, now)
}

func openBarrierForBoot(ctx context.Context, tx *sql.Tx, deviceID, bootID string, now time.Time) error {
	var currentBoot, status string
	if err := tx.QueryRowContext(ctx, `SELECT boot_id, status FROM device_reconciliation WHERE device_id = ?`, deviceID).Scan(&currentBoot, &status); err != nil {
		return fmt.Errorf("load reconciliation barrier: %w", err)
	}
	if currentBoot != bootID {
		return fmt.Errorf("reconciliation boot %q does not match current device boot %q", bootID, currentBoot)
	}
	if status == "required" {
		return nil
	}
	return markBarrierRequired(ctx, tx, deviceID, bootID, now)
}

func markBarrierRequired(ctx context.Context, tx *sql.Tx, deviceID, bootID string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE device_reconciliation SET status = 'required',
			last_resolution_status = NULL, resolution_evidence_json = NULL,
			resolution_sha256 = NULL, resolved_at = NULL, updated_at = ?
		WHERE device_id = ? AND boot_id = ?`,
		formatRuntimeTime(now), deviceID, bootID); err != nil {
		return fmt.Errorf("open reconciliation barrier: %w", err)
	}
	return nil
}
