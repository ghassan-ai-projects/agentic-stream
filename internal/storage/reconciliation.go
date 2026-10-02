package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// Resolve records state/feedback evidence for the device barrier. Command
// outcomes must already have gone through Dispatcher.ReconcileUnknown;
// unresolved command ledgers prevent a barrier clear. `manual_review` leaves
// the device barrier open.
func (s *ReconciliationStore) Resolve(ctx context.Context, deviceID, bootID, finalStatus string, evidence map[string]any, authorityEpoch, ownerInstance string) (bool, error) {
	if finalStatus != "succeeded" && finalStatus != "failed" && finalStatus != "manual_review" {
		return false, fmt.Errorf("invalid reconciliation status %q", finalStatus)
	}
	if s == nil || s.DB == nil || s.Authority == nil || !allNonEmpty(deviceID, bootID, authorityEpoch, ownerInstance) || len(evidence) == 0 {
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
		if err := assertResolvableBarrier(ctx, tx, deviceID, bootID, evidence); err != nil {
			return err
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

// assertResolvableBarrier requires an open barrier for this boot and evidence
// whose typed state matches both its own digest and the latest bound device
// state, with no command outcome still awaiting dispatcher reconciliation.
func assertResolvableBarrier(ctx context.Context, tx *sql.Tx, deviceID, bootID string, evidence map[string]any) error {
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
	unresolved, err := countUnresolvedCommands(ctx, tx, deviceID, bootID)
	if err != nil {
		return err
	}
	if unresolved > 0 {
		return fmt.Errorf("cannot clear device barrier while %d command outcomes still require dispatcher reconciliation", unresolved)
	}
	return nil
}

func allNonEmpty(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return false
		}
	}
	return true
}

func countUnresolvedCommands(ctx context.Context, tx *sql.Tx, deviceID, bootID string) (int64, error) {
	var unresolved int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM commands AS c
		JOIN device_command_bindings AS b ON b.command_id = c.command_id
		WHERE b.device_id = ? AND b.boot_id = ?
			AND c.status IN ('outcome_unknown', 'reconciling', 'manual_review')`, deviceID, bootID).Scan(&unresolved); err != nil {
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
