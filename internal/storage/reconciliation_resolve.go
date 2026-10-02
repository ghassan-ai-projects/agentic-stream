package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

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
