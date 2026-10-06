package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

// AppendDispatchNotice publishes the durable fact that a dispatch result was recorded.
func (tx *Tx) AppendDispatchNotice(ctx context.Context, n domain.DispatchNotice) error {
	command := n.Command
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, notify.LifecycleEvent{
		ID:           "command.dispatched:" + command.CommandID + ":" + n.OutcomeID,
		TenantID:     command.TenantID,
		Subject:      "command/" + command.CommandID,
		PartitionKey: command.CommandID,
		Payload:      notify.CommandDispatched{CommandID: command.CommandID, IntentID: command.IntentID, OutcomeID: n.OutcomeID, Status: n.Status},
		At:           n.At,
		Trace:        n.Trace,
	}); err != nil {
		return fmt.Errorf("append command dispatched notification: %w", err)
	}
	return nil
}

// AppendOutcomeNotice publishes the durable fact that an outcome was recorded.
func (tx *Tx) AppendOutcomeNotice(ctx context.Context, n domain.OutcomeNotice) error {
	command := n.Command
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, notify.LifecycleEvent{
		ID:           "outcome.recorded:" + n.OutcomeID,
		TenantID:     command.TenantID,
		Subject:      "outcome/" + n.OutcomeID,
		PartitionKey: command.CommandID,
		Payload:      outcomeRecorded(n),
		At:           n.At,
		Trace:        n.Trace,
	}); err != nil {
		return fmt.Errorf("append outcome recorded notification: %w", err)
	}
	return nil
}

func outcomeRecorded(n domain.OutcomeNotice) notify.OutcomeRecorded {
	return notify.OutcomeRecorded{
		IntentID: n.Command.IntentID, CommandID: n.Command.CommandID, OutcomeID: n.OutcomeID,
		OutcomeDigest: canonicaljson.EncodeDigest(n.Digest),
		Status:        domain.NotifiedStatus(n.Status), ReconciliationStatus: n.Reconciliation,
	}
}

// AppendReconciliationNotice publishes the durable fact that independent
// evidence settled an outcome.
func (tx *Tx) AppendReconciliationNotice(ctx context.Context, n domain.ReconciliationNotice) error {
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, notify.LifecycleEvent{
		ID:           "outcome.reconciled:" + n.OutcomeID,
		TenantID:     n.TenantID,
		Subject:      "outcome/" + n.OutcomeID,
		PartitionKey: n.CommandID,
		Payload:      outcomeReconciled(n),
		At:           n.At,
		Trace:        n.Provenance.Trace,
	}); err != nil {
		return fmt.Errorf("append outcome.reconciled notification: %w", err)
	}
	return nil
}

func outcomeReconciled(n domain.ReconciliationNotice) notify.OutcomeReconciled {
	return notify.OutcomeReconciled{
		IntentID: n.IntentID, CommandID: n.CommandID, OutcomeID: n.OutcomeID,
		OutcomeDigest: canonicaljson.EncodeDigest(n.Provenance.Digest),
		FinalStatus:   n.FinalStatus, ReconciliationStatus: domain.ReconciliationReconciled,
		Verdict: domain.Verdict(n.FinalStatus), ReconciliationVersion: n.Provenance.Version,
	}
}
