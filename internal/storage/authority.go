package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		var existing TargetClaim
		var fence int64
		var leaseUntil, status string
		err := tx.QueryRowContext(ctx, `
			SELECT device_id, owner_epoch, owner_instance, boot_id, claim_fence,
			       lease_until, status
			FROM device_target_claims WHERE target = ?`, claim.Target).Scan(
			&existing.DeviceID, &existing.AuthorityEpoch, &existing.OwnerInstance,
			&existing.BootID, &fence, &leaseUntil, &status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load target claim: %w", err)
		}
		if err == nil {
			existing.Target = claim.Target
			expires, parseErr := time.Parse(time.RFC3339Nano, leaseUntil)
			if parseErr != nil {
				return fmt.Errorf("parse target claim lease: %w", parseErr)
			}
			owned := existing.AuthorityEpoch == claim.AuthorityEpoch && existing.OwnerInstance == claim.OwnerInstance
			if status == "active" && expires.After(now) && !owned {
				if err := appendAuthorityEventTx(ctx, tx, claim, "claim_rejected", map[string]any{
					"reason": "unexpired_owner", "current_epoch": existing.AuthorityEpoch,
					"current_instance": existing.OwnerInstance,
				}, now); err != nil {
					return err
				}
				claimError = ErrTargetClaimBusy
				return nil
			}
			if !owned || status != "active" || !expires.After(now) {
				fence++
			}
		} else {
			fence = 1
		}
		leaseUntil = formatRuntimeTime(now.Add(a.leaseDuration()))
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
			claim.BootID, fence, leaseUntil, formatRuntimeTime(now)); err != nil {
			return fmt.Errorf("write target claim: %w", err)
		}
		eventType := "claim_acquired"
		if existing.AuthorityEpoch == claim.AuthorityEpoch && existing.OwnerInstance == claim.OwnerInstance && status == "active" {
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

// BindCommand records the device and boot identity for a command immediately
// before transport delivery. A repeated identical binding is idempotent; a
// conflicting binding is rejected before any bytes are sent.
func (a *TargetAuthority) BindCommand(ctx context.Context, binding CommandBinding) error {
	if a == nil || a.DB == nil || binding.CommandID == "" || binding.Target == "" || binding.DeviceID == "" || binding.BootID == "" || binding.AuthorityEpoch == "" || binding.OwnerInstance == "" {
		return fmt.Errorf("command binding is not configured")
	}
	if a.InstanceID != "" && a.InstanceID != binding.OwnerInstance {
		return fmt.Errorf("command binding owner instance does not match authority")
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOrdinaryTx(ctx, tx, binding.AuthorityEpoch); err != nil {
			return err
		}
		var target, deviceID, bootID, epoch, instance string
		var storedDigest []byte
		err := tx.QueryRowContext(ctx, `SELECT target, device_id, boot_id, owner_epoch, owner_instance, command_sha256
			FROM device_command_bindings WHERE command_id = ?`, binding.CommandID).Scan(&target, &deviceID, &bootID, &epoch, &instance, &storedDigest)
		if err == nil {
			if target != binding.Target || deviceID != binding.DeviceID || bootID != binding.BootID || epoch != binding.AuthorityEpoch || instance != binding.OwnerInstance || !digestBytesMatch(storedDigest, binding.CommandDigest) {
				return fmt.Errorf("command %q is already bound to a different device lifetime", binding.CommandID)
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load command binding: %w", err)
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

// Resolve records state/feedback evidence for the device barrier. Command
// outcomes must already have gone through Dispatcher.ReconcileUnknown;
// unresolved command ledgers prevent a barrier clear. `manual_review` leaves
// the device barrier open.
func (s *ReconciliationStore) Resolve(ctx context.Context, deviceID, bootID, finalStatus string, evidence map[string]any, authorityEpoch, ownerInstance string) (bool, error) {
	if finalStatus != "succeeded" && finalStatus != "failed" && finalStatus != "manual_review" {
		return false, fmt.Errorf("invalid reconciliation status %q", finalStatus)
	}
	if s == nil || s.DB == nil || s.Authority == nil || deviceID == "" || bootID == "" || authorityEpoch == "" || ownerInstance == "" || len(evidence) == 0 {
		return false, fmt.Errorf("device, boot, authority, and reconciliation evidence are required")
	}
	if err := s.Authority.AssertRuntime(ctx, authorityEpoch); err != nil {
		return false, fmt.Errorf("assert authority before resolving device reconciliation: %w", err)
	}
	if err := ValidateDeviceReconciliationEvidence(evidence, deviceID, bootID); err != nil {
		return false, err
	}
	evidenceJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		return false, fmt.Errorf("canonicalize reconciliation evidence: %w", err)
	}
	evidenceHash := sha256.Sum256(evidenceJSON)
	now := s.now()
	cleared := finalStatus != "manual_review"
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.Authority.assertOrdinaryTx(ctx, tx, authorityEpoch); err != nil {
			return err
		}
		var status, currentBoot string
		var currentStateHash []byte
		if err := tx.QueryRowContext(ctx, `SELECT status, boot_id, state_sha256 FROM device_reconciliation WHERE device_id = ?`, deviceID).Scan(&status, &currentBoot, &currentStateHash); err != nil {
			return fmt.Errorf("load reconciliation barrier: %w", err)
		}
		if status != "required" || currentBoot != bootID {
			return ErrReconciliationRequired
		}
		stateDigest, _ := evidence["state_digest"].(string)
		if stateDigest != "sha256:"+hex.EncodeToString(currentStateHash) {
			return fmt.Errorf("reconciliation evidence does not bind the latest device state")
		}
		stateJSON, err := canonicaljson.Marshal(evidence["state"])
		if err != nil {
			return fmt.Errorf("canonicalize reconciliation state evidence: %w", err)
		}
		stateHash := sha256.Sum256(stateJSON)
		if stateDigest != "sha256:"+hex.EncodeToString(stateHash[:]) {
			return fmt.Errorf("reconciliation state digest does not match typed state evidence")
		}
		if unresolved, err := countUnresolvedCommands(ctx, tx); err != nil {
			return err
		} else if unresolved > 0 {
			return fmt.Errorf("cannot clear device barrier while %d command outcomes still require dispatcher reconciliation", unresolved)
		}
		newStatus := "required"
		if cleared {
			newStatus = "clear"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE device_reconciliation SET status = ?, last_resolution_status = ?,
				resolution_evidence_json = ?, resolution_sha256 = ?, resolved_at = ?,
				updated_at = ? WHERE device_id = ? AND status = 'required' AND boot_id = ?`,
			newStatus, finalStatus, evidenceJSON, evidenceHash[:], formatRuntimeTime(now), formatRuntimeTime(now), deviceID, bootID); err != nil {
			return fmt.Errorf("resolve reconciliation barrier: %w", err)
		}
		return appendAuthorityEventTx(ctx, tx, TargetClaim{Target: deviceID, DeviceID: deviceID, BootID: bootID, AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance}, "reconciliation_recorded", map[string]any{
			"final_status": finalStatus, "evidence_sha256": "sha256:" + hex.EncodeToString(evidenceHash[:]), "barrier_cleared": cleared,
		}, now)
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	return cleared, nil
}

func countUnresolvedCommands(ctx context.Context, tx *sql.Tx) (int64, error) {
	var unresolved int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE status IN ('outcome_unknown', 'reconciling', 'manual_review')`).Scan(&unresolved); err != nil {
		return 0, fmt.Errorf("count unresolved command outcomes: %w", err)
	}
	return unresolved, nil
}

// ValidateDeviceReconciliationEvidence validates the typed, boot-bound
// evidence envelope used by the serial boundary and dispatcher. It verifies
// the supplied document references; it does not claim that an external
// feedback system actually observed a physical transition.
func ValidateDeviceReconciliationEvidence(evidence map[string]any, deviceID, bootID string) error {
	if len(evidence) == 0 || deviceID == "" || bootID == "" {
		return fmt.Errorf("device reconciliation evidence is required")
	}
	source, _ := evidence["source"].(string)
	if source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	evidenceType, _ := evidence["evidence_type"].(string)
	if evidenceType != "device_state_feedback" {
		return fmt.Errorf("reconciliation evidence_type must be device_state_feedback")
	}
	for _, key := range []string{"evidence_digest", "feedback_digest", "state_digest"} {
		digest, _ := evidence[key].(string)
		if !validSHA256Reference(digest) {
			return fmt.Errorf("reconciliation %s must be a sha256 reference", key)
		}
	}
	if evidenceDevice, _ := evidence["device_id"].(string); evidenceDevice != deviceID {
		return fmt.Errorf("reconciliation evidence device identity does not match the binding")
	}
	if evidenceBoot, _ := evidence["boot_id"].(string); evidenceBoot != bootID {
		return fmt.Errorf("reconciliation evidence boot identity does not match the binding")
	}
	state, ok := evidence["state"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include typed device state")
	}
	if stateDevice, _ := state["device_id"].(string); stateDevice != deviceID {
		return fmt.Errorf("reconciliation state device identity does not match the binding")
	}
	if stateBoot, _ := state["boot_id"].(string); stateBoot != bootID {
		return fmt.Errorf("reconciliation state boot identity does not match the binding")
	}
	feedback, ok := evidence["feedback"].(map[string]any)
	if !ok {
		return fmt.Errorf("reconciliation evidence must include independent feedback")
	}
	return verifyEvidenceReferences(evidence, feedback)
}

func verifyEvidenceReferences(evidence map[string]any, feedback map[string]any) error {
	withoutDigest := make(map[string]any, len(evidence)-1)
	for key, value := range evidence {
		if key != "evidence_digest" {
			withoutDigest[key] = value
		}
	}
	bundle, err := canonicaljson.Marshal(withoutDigest)
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation evidence bundle: %w", err)
	}
	bundleHash := sha256.Sum256(bundle)
	provided, _ := evidence["evidence_digest"].(string)
	if provided != "sha256:"+hex.EncodeToString(bundleHash[:]) {
		return fmt.Errorf("reconciliation evidence_digest does not match the evidence bundle")
	}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		return fmt.Errorf("canonicalize reconciliation feedback: %w", err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	provided, _ = evidence["feedback_digest"].(string)
	if provided != "sha256:"+hex.EncodeToString(feedbackHash[:]) {
		return fmt.Errorf("reconciliation feedback_digest does not match feedback")
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

// SafetyEvent is a durable input to the soak verdict. It is intentionally
// explicit; arbitrary telemetry labels cannot silently become safety claims.
type SafetyEvent struct {
	Type      string
	Target    string
	CommandID string
	Details   map[string]any
	Occurred  time.Time
}

// SafetyLedger records events from the emulator or physical evidence bridge.
// The runtime does not synthesize physical transitions from a receipt.
type SafetyLedger struct{ DB *DB }

// Record appends one validated safety event.
func (l *SafetyLedger) Record(ctx context.Context, event SafetyEvent) error {
	if l == nil || l.DB == nil || event.Target == "" || !validSafetyEventType(event.Type) {
		return fmt.Errorf("safety event type, target, and ledger are required")
	}
	if event.Occurred.IsZero() {
		event.Occurred = time.Now().UTC()
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	if event.Type == "physical_transition" && eventComplete(event.Details) && !PhysicalEvidenceComplete(event.Details) {
		return fmt.Errorf("complete physical transition evidence requires source and sha256 evidence_digest")
	}
	data, err := canonicaljson.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("canonicalize safety event: %w", err)
	}
	hash := sha256.Sum256(data)
	if _, err := l.DB.ExecContext(ctx, `
		INSERT INTO device_safety_events
			(event_type, target, command_id, details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`, event.Type, event.Target, nullableText(event.CommandID), data, hash[:], formatRuntimeTime(event.Occurred)); err != nil {
		return fmt.Errorf("record safety event: %w", err)
	}
	return nil
}

func validSafetyEventType(eventType string) bool {
	switch eventType {
	case "unsafe_output", "stale_energizing_effect", "duplicate_net_energizing_effect", "unexplained_actuator_transition", "false_verified_success", "safe_state_deadline_miss", "physical_transition":
		return true
	default:
		return false
	}
}

// PhysicalEvidenceComplete reports whether a physical transition carries the
// minimum provenance shape required for a complete run artifact. This validates
// evidence metadata, not the truth of the physical observation; an independent
// feedback system must still supply and own that evidence.
func PhysicalEvidenceComplete(details map[string]any) bool {
	if !eventComplete(details) {
		return false
	}
	source, _ := details["source"].(string)
	digest, _ := details["evidence_digest"].(string)
	return source != "" && validSHA256Reference(digest)
}

func eventComplete(details map[string]any) bool {
	complete, _ := details["evidence_complete"].(bool)
	return complete
}

func validSHA256Reference(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
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
