package store

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

// AppendDispatchNotice publishes the durable fact that a dispatch result was recorded.
func (tx *Tx) AppendDispatchNotice(ctx context.Context, n domain.DispatchNotice) error {
	command := n.Command
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "command.dispatched:"+command.CommandID+":"+n.OutcomeID, command.TenantID, notify.TypeCommandDispatched, "command/"+command.CommandID, command.CommandID, map[string]any{
		"tenant_id": command.TenantID, "command_id": command.CommandID, "intent_id": command.IntentID,
		"outcome_id": n.OutcomeID, "status": n.Status, "source_authority": notify.SourceForTenant(command.TenantID),
	}, n.At, n.Trace); err != nil {
		return fmt.Errorf("append command dispatched notification: %w", err)
	}
	return nil
}

// AppendOutcomeNotice publishes the durable fact that an outcome was recorded.
func (tx *Tx) AppendOutcomeNotice(ctx context.Context, n domain.OutcomeNotice) error {
	command := n.Command
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "outcome.recorded:"+n.OutcomeID, command.TenantID, notify.TypeOutcomeRecorded, "outcome/"+n.OutcomeID, command.CommandID, map[string]any{
		"tenant_id": command.TenantID, "outcome_id": n.OutcomeID, "command_id": command.CommandID, "status": domain.NotifiedStatus(n.Status),
		"reconciliation_status": n.Reconciliation, "intent_id": command.IntentID,
		"outcome_digest":   "sha256:" + hex.EncodeToString(n.Digest),
		"source_authority": notify.SourceForTenant(command.TenantID),
	}, n.At, n.Trace); err != nil {
		return fmt.Errorf("append outcome recorded notification: %w", err)
	}
	return nil
}

// AppendReconciliationNotice publishes the durable fact that independent
// evidence settled an outcome.
func (tx *Tx) AppendReconciliationNotice(ctx context.Context, n domain.ReconciliationNotice) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "outcome.reconciled:"+n.OutcomeID, n.TenantID, notify.TypeOutcomeReconciled, "outcome/"+n.OutcomeID, n.CommandID, map[string]any{
		"tenant_id": n.TenantID, "outcome_id": n.OutcomeID, "command_id": n.CommandID, "final_status": n.FinalStatus,
		"outcome_digest": "sha256:" + hex.EncodeToString(n.Provenance.Digest), "reconciliation_status": domain.ReconciliationReconciled, "intent_id": n.IntentID,
		"verdict": domain.Verdict(n.FinalStatus), "reconciliation_version": n.Provenance.Version,
		"source_authority": notify.SourceForTenant(n.TenantID),
	}, n.At, n.Provenance.Trace); err != nil {
		return fmt.Errorf("append outcome.reconciled notification: %w", err)
	}
	return nil
}
