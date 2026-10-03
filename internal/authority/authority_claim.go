package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

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
	err := tx.QueryRowContext(ctx, loadTargetClaimSQL, target).Scan(
		&current.DeviceID, &current.AuthorityEpoch, &current.OwnerInstance,
		&current.BootID, &current.fence, &leaseUntil, &current.status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load target claim: %w", err)
	}
	return parseTargetClaimLease(current, leaseUntil)
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

const loadTargetClaimSQL = `
		SELECT device_id, owner_epoch, owner_instance, boot_id, claim_fence,
		       lease_until, status
		FROM device_target_claims WHERE target = ?`

func parseTargetClaimLease(current *storedTargetClaim, leaseUntil string) (*storedTargetClaim, error) {
	var err error
	current.expires, err = time.Parse(time.RFC3339Nano, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("parse target claim lease: %w", err)
	}
	return current, nil
}
