package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

const loadClaimSQL = `
		SELECT device_id, boot_id, owner_epoch, owner_instance, claim_fence, lease_until, status
		FROM device_target_claims WHERE target = ?`

// LoadClaim returns the claim held on target, or nil when the target has
// never been claimed.
func (t *Tx) LoadClaim(ctx context.Context, target string) (*domain.HeldClaim, error) {
	held := domain.HeldClaim{TargetClaim: domain.TargetClaim{Target: target}}
	var leaseUntil, status string
	err := t.tx.QueryRowContext(ctx, loadClaimSQL, target).Scan(&held.Device.DeviceID, &held.Device.BootID,
		&held.Owner.Epoch, &held.Owner.Instance, &held.Fence, &leaseUntil, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load target claim: %w", err)
	}
	held.Status = domain.ClaimStatus(status)
	if held.LeaseUntil, err = parseTime("target claim lease", leaseUntil); err != nil {
		return nil, err
	}
	return &held, nil
}

const writeClaimSQL = `
		INSERT INTO device_target_claims
			(target, device_id, owner_epoch, owner_instance, boot_id, claim_fence, lease_until, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target) DO UPDATE SET
			device_id = excluded.device_id,
			owner_epoch = excluded.owner_epoch,
			owner_instance = excluded.owner_instance,
			boot_id = excluded.boot_id,
			claim_fence = excluded.claim_fence,
			lease_until = excluded.lease_until,
			status = excluded.status,
			updated_at = excluded.updated_at`

// WriteClaim records claim as the active claim on its target with fence and
// lease.
func (t *Tx) WriteClaim(ctx context.Context, claim domain.TargetClaim, fence int64, leaseUntil, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, writeClaimSQL,
		claim.Target, claim.Device.DeviceID, claim.Owner.Epoch, claim.Owner.Instance, claim.Device.BootID,
		fence, formatTime(leaseUntil), string(domain.ClaimActive), formatTime(now)); err != nil {
		return fmt.Errorf("write target claim: %w", err)
	}
	return nil
}

// MarkClaimReleased records the claim on target as released.
func (t *Tx) MarkClaimReleased(ctx context.Context, target string, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, `UPDATE device_target_claims SET status = ?, updated_at = ? WHERE target = ?`,
		string(domain.ClaimReleased), formatTime(now), target); err != nil {
		return fmt.Errorf("release target claim: %w", err)
	}
	return nil
}
