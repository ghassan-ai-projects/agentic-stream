package authority

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

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
	claim := TargetClaim{Target: deviceID, DeviceID: deviceID, BootID: bootID, AuthorityEpoch: authorityEpoch, OwnerInstance: ownerInstance}
	evidenceJSON, err := s.authorizeResolution(ctx, claim, evidence)
	if err != nil {
		return false, err
	}
	resolution := barrierResolution{claim: claim, finalStatus: finalStatus, evidence: evidence, evidenceJSON: evidenceJSON, now: s.now()}
	return s.recordResolution(ctx, resolution)
}

func (s *ReconciliationStore) authorizeResolution(ctx context.Context, claim TargetClaim, evidence map[string]any) ([]byte, error) {
	if err := s.Authority.AssertRuntime(ctx, claim.AuthorityEpoch); err != nil {
		return nil, fmt.Errorf("assert authority before resolving device reconciliation: %w", err)
	}
	if err := ValidateDeviceReconciliationEvidence(evidence, claim.DeviceID, claim.BootID); err != nil {
		return nil, err
	}
	evidenceJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("canonicalize reconciliation evidence: %w", err)
	}
	return evidenceJSON, nil
}

func (s *ReconciliationStore) recordResolution(ctx context.Context, resolution barrierResolution) (bool, error) {
	if err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return s.resolveTx(ctx, tx, resolution)
	}); err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	return resolution.clears(), nil
}

// barrierResolution is one recorded resolution of a device barrier.
type barrierResolution struct {
	claim        TargetClaim
	finalStatus  string
	evidence     map[string]any
	evidenceJSON []byte
	now          time.Time
}

// clears reports whether the resolution clears the barrier; manual review
// leaves it open.
func (r barrierResolution) clears() bool {
	return r.finalStatus != "manual_review"
}

func (s *ReconciliationStore) resolveTx(ctx context.Context, tx *sql.Tx, r barrierResolution) error {
	if err := s.Authority.assertOrdinaryTx(ctx, tx, r.claim.AuthorityEpoch); err != nil {
		return err
	}
	if err := assertResolvableBarrier(ctx, tx, r.claim.DeviceID, r.claim.BootID, r.evidence); err != nil {
		return err
	}
	return recordResolvedBarrier(ctx, tx, r)
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
	if err := checkLatestStateEvidence(evidence, currentStateHash); err != nil {
		return err
	}
	return requireReconciledCommands(ctx, tx, deviceID, bootID)
}

func checkLatestStateEvidence(evidence map[string]any, currentStateHash []byte) error {
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
	return nil
}

func requireReconciledCommands(ctx context.Context, tx *sql.Tx, deviceID, bootID string) error {
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

func recordResolvedBarrier(ctx context.Context, tx *sql.Tx, r barrierResolution) error {
	newStatus := "required"
	if r.clears() {
		newStatus = "clear"
	}
	evidenceHash := sha256.Sum256(r.evidenceJSON)
	if _, err := tx.ExecContext(ctx, recordResolvedDeviceBarrierSQL,
		newStatus, r.finalStatus, r.evidenceJSON, evidenceHash[:], formatRuntimeTime(r.now), formatRuntimeTime(r.now), r.claim.DeviceID, r.claim.BootID); err != nil {
		return fmt.Errorf("resolve reconciliation barrier: %w", err)
	}
	return appendAuthorityEventTx(ctx, tx, r.claim, "reconciliation_recorded", map[string]any{
		"final_status": r.finalStatus, "evidence_sha256": "sha256:" + hex.EncodeToString(evidenceHash[:]), "barrier_cleared": r.clears(),
	}, r.now)
}

const recordResolvedDeviceBarrierSQL = `
		UPDATE device_reconciliation SET status = ?, last_resolution_status = ?,
			resolution_evidence_json = ?, resolution_sha256 = ?, resolved_at = ?,
			updated_at = ? WHERE device_id = ? AND status = 'required' AND boot_id = ?`
