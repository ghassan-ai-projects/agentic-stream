package actions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

// reconcilableCommand is a command awaiting reconciliation.
type reconcilableCommand struct {
	commandID, tenantID, intentID, target string
	traceparent, tracestate               sql.NullString
}

const insertReconciledOutcomeSQL = `
		INSERT INTO outcomes (
			outcome_id, command_id, ordinal, status, provider_result_json,
			observed_effect_json, reconciliation_status, outcome_sha256, traceparent, tracestate, occurred_at
		) VALUES (?, ?, (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM outcomes WHERE command_id = ?), 'reconciled', ?, NULL, 'reconciled', ?, ?, ?, ?)`

const updateReconciledVerificationSQL = `
		UPDATE verifications SET outcome_id = ?, status = ?, reconciled_at = ?, updated_at = ?
		WHERE command_id = ?`

const loadReconciledProvenanceSQL = `
		SELECT o.ordinal, o.outcome_sha256, d.traceparent, d.tracestate
		FROM outcomes o
		JOIN commands c ON c.command_id = o.command_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE o.outcome_id = ? AND o.command_id = ? AND i.intent_id = ?`

type reconciledProvenance struct {
	version int
	digest  []byte
	trace   contractsv1.TraceContext
}

// ReconcileUnknown closes an outcome_unknown command using independently
// observed provider evidence. It is the only path that may resolve a command
// after an effector call whose result was uncertain.
func (d *Dispatcher) ReconcileUnknown(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	if finalStatus != "succeeded" && finalStatus != "failed" && finalStatus != "manual_review" {
		return fmt.Errorf("invalid reconciliation status %q", finalStatus)
	}
	if err := validateReconciliationEvidence(evidence); err != nil {
		return err
	}
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		return d.reconcileUnknownTx(ctx, tx, commandID, finalStatus, evidence)
	}); err != nil {
		return fmt.Errorf("reconcile unknown command: %w", err)
	}
	return nil
}

func validateReconciliationEvidence(evidence map[string]any) error {
	if len(evidence) == 0 {
		return fmt.Errorf("reconciliation evidence is required")
	}
	if source, _ := evidence["source"].(string); source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	evidenceType, _ := evidence["evidence_type"].(string)
	if evidenceType != "provider_observation" && evidenceType != "device_state_feedback" {
		return fmt.Errorf("reconciliation evidence_type is required")
	}
	return validateTypedReconciliationEvidence(evidence, evidenceType)
}

func validateTypedReconciliationEvidence(evidence map[string]any, evidenceType string) error {
	if evidenceType == "device_state_feedback" {
		if err := requireDeviceFeedbackFields(evidence); err != nil {
			return err
		}
	}
	return requireEvidenceDigest(evidence)
}

func requireDeviceFeedbackFields(evidence map[string]any) error {
	for _, key := range []string{"device_id", "boot_id", "state", "feedback_digest"} {
		if _, ok := evidence[key]; !ok {
			return fmt.Errorf("device reconciliation evidence requires %s", key)
		}
	}
	return nil
}

// requireEvidenceDigest validates the first present evidence, state, or
// feedback digest, and requires at least one.
func requireEvidenceDigest(evidence map[string]any) error {
	for _, key := range []string{"evidence_digest", "state_digest", "feedback_digest"} {
		digest, ok := evidence[key].(string)
		if !ok || digest == "" {
			continue
		}
		if _, err := canonicaljson.DecodeDigest(digest); err != nil {
			return fmt.Errorf("invalid reconciliation %s: %w", key, err)
		}
		return nil
	}
	return fmt.Errorf("reconciliation evidence must include a sha256 evidence, state, or feedback digest")
}

func (d *Dispatcher) reconcileUnknownTx(ctx context.Context, tx *sql.Tx, commandID, finalStatus string, evidence map[string]any) error {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return err
	}
	command, err := loadReconcilableCommand(ctx, tx, commandID)
	if err != nil {
		return err
	}
	if err := verifyDeviceBinding(ctx, tx, command, evidence); err != nil {
		return err
	}
	return d.recordReconciliation(ctx, tx, command, finalStatus, evidence)
}

func loadReconcilableCommand(ctx context.Context, tx *sql.Tx, commandID string) (reconcilableCommand, error) {
	command := reconcilableCommand{commandID: commandID}
	var currentStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT c.status, c.tenant_id, c.intent_id, c.normalized_target, d.traceparent, d.tracestate
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE c.command_id = ?`, commandID).Scan(&currentStatus, &command.tenantID, &command.intentID, &command.target, &command.traceparent, &command.tracestate); err != nil {
		return reconcilableCommand{}, fmt.Errorf("load command %s for reconciliation: %w", commandID, err)
	}
	if currentStatus != "reconciling" && currentStatus != "outcome_unknown" && currentStatus != "manual_review" {
		return reconcilableCommand{}, fmt.Errorf("command %s is not awaiting reconciliation", commandID)
	}
	return command, nil
}

// verifyDeviceBinding requires evidence for a device-bound command to name
// the bound target and carry valid boot-bound device evidence.
func verifyDeviceBinding(ctx context.Context, tx *sql.Tx, command reconcilableCommand, evidence map[string]any) error {
	if err := deviceauthority.VerifyCommandEvidence(ctx, tx, deviceauthority.CommandEvidence{
		CommandID: command.commandID, Target: command.target, Evidence: evidence,
	}); err != nil {
		return fmt.Errorf("verify device command evidence: %w", err)
	}
	return nil
}

func (d *Dispatcher) recordReconciliation(ctx context.Context, tx *sql.Tx, command reconcilableCommand, finalStatus string, evidence map[string]any) error {
	now := d.clk.Now().UTC()
	outcomeID := d.idGen.New(ids.PrefixOutcome)
	outcomeSHA, err := outcomeDigest(map[string]any{
		"outcome_id": outcomeID, "command_id": command.commandID, "status": "reconciled",
		"observed_at": formatTime(now),
		"result":      map[string]any{"final_status": finalStatus, "evidence": evidence},
	})
	if err != nil {
		return fmt.Errorf("reconciliation outcome: %w", err)
	}
	return storeReconciliation(ctx, tx, command, finalStatus, evidence, outcomeID, outcomeSHA, now)
}

func storeReconciliation(ctx context.Context, tx *sql.Tx, command reconcilableCommand, finalStatus string, evidence map[string]any, outcomeID string, outcomeSHA []byte, now time.Time) error {
	evidenceJSON, err := optionalJSON(evidence)
	if err != nil {
		return fmt.Errorf("encode reconciliation evidence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, insertReconciledOutcomeSQL,
		outcomeID, command.commandID, command.commandID, evidenceJSON, outcomeSHA, command.traceparent, command.tracestate, formatTime(now)); err != nil {
		return fmt.Errorf("record reconciliation outcome: %w", err)
	}
	return closeReconciliation(ctx, tx, command, finalStatus, outcomeID, now)
}

func closeReconciliation(ctx context.Context, tx *sql.Tx, command reconcilableCommand, finalStatus, outcomeID string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status IN ('reconciling', 'outcome_unknown')", finalStatus, formatTime(now), command.commandID); err != nil {
		return fmt.Errorf("close reconciled command: %w", err)
	}
	if _, err := tx.ExecContext(ctx, updateReconciledVerificationSQL, outcomeID, reconciledVerificationStatus(finalStatus), formatTime(now), formatTime(now), command.commandID); err != nil {
		return fmt.Errorf("update reconciliation verification: %w", err)
	}
	if err := appendOutcomeReconciledNotification(ctx, tx, command.tenantID, command.intentID, command.commandID, outcomeID, finalStatus, now); err != nil {
		return fmt.Errorf("append outcome reconciled notification: %w", err)
	}
	return nil
}

func reconciledVerificationStatus(finalStatus string) string {
	switch finalStatus {
	case "succeeded":
		return "reconciled"
	case "failed":
		return "refuted"
	default:
		return "inconclusive"
	}
}

func appendOutcomeReconciledNotification(ctx context.Context, tx *sql.Tx, tenantID, intentID, commandID, outcomeID, finalStatus string, now time.Time) error {
	provenance, err := loadReconciledProvenance(ctx, tx, outcomeID, commandID, intentID)
	if err != nil {
		return err
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "outcome.reconciled:"+outcomeID, tenantID, notify.TypeOutcomeReconciled, "outcome/"+outcomeID, commandID, map[string]any{
		"tenant_id": tenantID, "outcome_id": outcomeID, "command_id": commandID, "final_status": finalStatus,
		"outcome_digest": "sha256:" + hex.EncodeToString(provenance.digest), "reconciliation_status": "reconciled", "intent_id": intentID,
		"verdict": outcomeVerdict(finalStatus), "reconciliation_version": provenance.version,
		"source_authority": notify.SourceForTenant(tenantID),
	}, now, contractsv1.TraceContext{Traceparent: provenance.trace.Traceparent, Tracestate: provenance.trace.Tracestate}); err != nil {
		return fmt.Errorf("append outcome.reconciled notification: %w", err)
	}
	return nil
}

func loadReconciledProvenance(ctx context.Context, tx *sql.Tx, outcomeID, commandID, intentID string) (reconciledProvenance, error) {
	var reconciliationVersion int
	var outcomeSHA []byte
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, loadReconciledProvenanceSQL, outcomeID, commandID, intentID).
		Scan(&reconciliationVersion, &outcomeSHA, &traceparent, &tracestate); err != nil {
		return reconciledProvenance{}, fmt.Errorf("load reconciled outcome provenance: %w", err)
	}
	if reconciliationVersion < 1 || len(outcomeSHA) != sha256.Size {
		return reconciledProvenance{}, fmt.Errorf("reconciled outcome provenance is incomplete")
	}
	return reconciledProvenance{reconciliationVersion, outcomeSHA, contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}}, nil
}

func outcomeVerdict(finalStatus string) string {
	switch finalStatus {
	case "succeeded":
		return "verified"
	case "failed":
		return "refuted"
	default:
		return "inconclusive"
	}
}
