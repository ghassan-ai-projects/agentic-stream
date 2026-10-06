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
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, dispatchedEvent(n)); err != nil {
		return fmt.Errorf("append command dispatched notification: %w", err)
	}
	return nil
}

func dispatchedEvent(n domain.DispatchNotice) notify.LifecycleEvent {
	command := n.Command
	return notify.LifecycleEvent{
		ID:           "command.dispatched:" + command.CommandID + ":" + n.OutcomeID,
		TenantID:     command.TenantID,
		Type:         notify.TypeCommandDispatched,
		Subject:      "command/" + command.CommandID,
		PartitionKey: command.CommandID,
		Data: map[string]any{
			"tenant_id": command.TenantID, "command_id": command.CommandID, "intent_id": command.IntentID,
			"outcome_id": n.OutcomeID, "status": n.Status, "source_authority": notify.SourceForTenant(command.TenantID),
		},
		At:    n.At,
		Trace: n.Trace,
	}
}

// AppendOutcomeNotice publishes the durable fact that an outcome was recorded.
func (tx *Tx) AppendOutcomeNotice(ctx context.Context, n domain.OutcomeNotice) error {
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, outcomeRecordedEvent(n)); err != nil {
		return fmt.Errorf("append outcome recorded notification: %w", err)
	}
	return nil
}

func outcomeRecordedEvent(n domain.OutcomeNotice) notify.LifecycleEvent {
	command := n.Command
	return notify.LifecycleEvent{
		ID:           "outcome.recorded:" + n.OutcomeID,
		TenantID:     command.TenantID,
		Type:         notify.TypeOutcomeRecorded,
		Subject:      "outcome/" + n.OutcomeID,
		PartitionKey: command.CommandID,
		Data:         outcomeRecordedData(n),
		At:           n.At,
		Trace:        n.Trace,
	}
}

func outcomeRecordedData(n domain.OutcomeNotice) map[string]any {
	command := n.Command
	return map[string]any{
		"tenant_id": command.TenantID, "outcome_id": n.OutcomeID, "command_id": command.CommandID, "status": domain.NotifiedStatus(n.Status),
		"reconciliation_status": n.Reconciliation, "intent_id": command.IntentID,
		"outcome_digest":   "sha256:" + hex.EncodeToString(n.Digest),
		"source_authority": notify.SourceForTenant(command.TenantID),
	}
}

// AppendReconciliationNotice publishes the durable fact that independent
// evidence settled an outcome.
func (tx *Tx) AppendReconciliationNotice(ctx context.Context, n domain.ReconciliationNotice) error {
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, outcomeReconciledEvent(n)); err != nil {
		return fmt.Errorf("append outcome.reconciled notification: %w", err)
	}
	return nil
}

func outcomeReconciledEvent(n domain.ReconciliationNotice) notify.LifecycleEvent {
	return notify.LifecycleEvent{
		ID:           "outcome.reconciled:" + n.OutcomeID,
		TenantID:     n.TenantID,
		Type:         notify.TypeOutcomeReconciled,
		Subject:      "outcome/" + n.OutcomeID,
		PartitionKey: n.CommandID,
		Data: map[string]any{
			"tenant_id": n.TenantID, "outcome_id": n.OutcomeID, "command_id": n.CommandID, "final_status": n.FinalStatus,
			"outcome_digest": "sha256:" + hex.EncodeToString(n.Provenance.Digest), "reconciliation_status": domain.ReconciliationReconciled, "intent_id": n.IntentID,
			"verdict": domain.Verdict(n.FinalStatus), "reconciliation_version": n.Provenance.Version,
			"source_authority": notify.SourceForTenant(n.TenantID),
		},
		At:    n.At,
		Trace: n.Provenance.Trace,
	}
}
