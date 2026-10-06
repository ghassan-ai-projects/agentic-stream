package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

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

// LoadReconcilableCommand reads a command and its decision trace context.
func (tx *Tx) LoadReconcilableCommand(ctx context.Context, commandID string) (domain.ReconcilableCommand, error) {
	command := domain.ReconcilableCommand{ID: commandID}
	var traceparent, tracestate sql.NullString
	if err := tx.tx.QueryRowContext(ctx, `
		SELECT c.status, c.tenant_id, c.intent_id, c.normalized_target, d.traceparent, d.tracestate
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE c.command_id = ?`, commandID).Scan(&command.Status, &command.TenantID, &command.IntentID, &command.Target, &traceparent, &tracestate); err != nil {
		return domain.ReconcilableCommand{}, fmt.Errorf("load command %s for reconciliation: %w", commandID, err)
	}
	command.Trace = contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}
	return command, nil
}

// VerifyDeviceBinding runs the device authority's command-evidence check on
// this transaction.
func (tx *Tx) VerifyDeviceBinding(ctx context.Context, command domain.ReconcilableCommand, evidence map[string]any) error {
	if err := deviceauthority.VerifyCommandEvidence(ctx, tx.tx, deviceauthority.CommandEvidence{
		CommandID: command.ID, Target: command.Target, Evidence: evidence,
	}); err != nil {
		return fmt.Errorf("verify device command evidence: %w", err)
	}
	return nil
}

// CloseReconciliation moves the reconciled command and its verification row to
// the states the final status implies.
func (tx *Tx) CloseReconciliation(ctx context.Context, closure domain.ReconciliationClosure) error {
	at := formatTime(closure.At)
	if _, err := tx.tx.ExecContext(ctx, "UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status IN ('reconciling', 'outcome_unknown', 'manual_review')", closure.FinalStatus, at, closure.Command.ID); err != nil {
		return fmt.Errorf("close reconciled command: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, updateReconciledVerificationSQL, closure.OutcomeID, domain.ReconciledVerificationStatus(closure.FinalStatus), at, at, closure.Command.ID); err != nil {
		return fmt.Errorf("update reconciliation verification: %w", err)
	}
	return nil
}

// LoadReconciledProvenance reads the stored outcome a reconciliation
// notification cites.
func (tx *Tx) LoadReconciledProvenance(ctx context.Context, outcomeID, commandID, intentID string) (domain.ReconciledProvenance, error) {
	var provenance domain.ReconciledProvenance
	var traceparent, tracestate sql.NullString
	if err := tx.tx.QueryRowContext(ctx, loadReconciledProvenanceSQL, outcomeID, commandID, intentID).
		Scan(&provenance.Version, &provenance.Digest, &traceparent, &tracestate); err != nil {
		return domain.ReconciledProvenance{}, fmt.Errorf("load reconciled outcome provenance: %w", err)
	}
	provenance.Trace = contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}
	return provenance, nil
}
