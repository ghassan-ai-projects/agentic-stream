package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// AppendApprovalWithdrawn publishes stale approval withdrawal evidence.
func (tx *Tx) AppendApprovalWithdrawn(ctx context.Context, event domain.ApprovalEvent) error {
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, approvalWithdrawnEvent(event)); err != nil {
		return fmt.Errorf("append approval withdrawn notification: %w", err)
	}
	return nil
}

func approvalWithdrawnEvent(event domain.ApprovalEvent) notify.LifecycleEvent {
	payload := notify.ApprovalWithdrawn{
		ApprovalID: event.ID, IntentID: event.Intent.IntentID, SituationID: event.Intent.SituationID,
		SituationVersion: event.Intent.SituationVersion, Reason: event.Reason,
	}
	return notify.ApprovalWithdrawnEvent(event.Intent.TenantID, payload, event.Now, traceContext(event.Intent))
}

// AppendApprovalResolved publishes a durable approval disposition.
func (tx *Tx) AppendApprovalResolved(ctx context.Context, event domain.ApprovalEvent) error {
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, approvalResolvedEvent(event)); err != nil {
		return fmt.Errorf("append approval resolved notification: %w", err)
	}
	return nil
}

func approvalResolvedEvent(event domain.ApprovalEvent) notify.LifecycleEvent {
	payload := notify.ApprovalResolved{
		ApprovalID: event.ID, IntentID: event.Intent.IntentID, DecisionID: event.Intent.DecisionID,
		SituationID: event.Intent.SituationID, SituationVersion: event.Intent.SituationVersion,
		Status: event.Status, Reason: event.Reason,
	}
	return notify.ApprovalResolvedEvent(event.Intent.TenantID, payload, event.Now, traceContext(event.Intent))
}

func traceContext(row domain.IntentRecord) contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: row.Traceparent, Tracestate: row.Tracestate}
}

// NotificationSource preserves the shared tenant lifecycle-event source.
func (tx *Tx) NotificationSource(tenant string) string { return notify.SourceForTenant(tenant) }

// AppendApprovalRequested publishes the sealed request notification. The
// notification must already be bound to the row's tenant and its source.
func (tx *Tx) AppendApprovalRequested(ctx context.Context, row domain.IntentRecord, request domain.ApprovalRequest, now time.Time) error {
	if err := request.Data.CheckBinding(row.TenantID, notify.SourceForTenant(row.TenantID)); err != nil {
		return fmt.Errorf("append approval requested notification: %w", err)
	}
	event := notify.ApprovalRequestedEvent(row.TenantID, approvalRequested(request.Data), now, traceContext(row))
	if err := notify.AppendLifecycleEvent(ctx, tx.tx, event); err != nil {
		return fmt.Errorf("append approval requested notification: %w", err)
	}
	return nil
}

func approvalRequested(n domain.ApprovalNotification) notify.ApprovalRequested {
	return notify.ApprovalRequested{
		ApprovalID: n.ApprovalID, IntentID: n.IntentID, DecisionID: n.DecisionID,
		SituationID: n.SituationID, SituationVersion: n.SituationVersion,
		IntentDigest: n.IntentDigest, SnapshotDigest: n.SnapshotDigest, RiskClass: n.RiskClass,
		ExpiresAt: n.ExpiresAt, Audience: n.Audience, Summary: n.Summary, Delta: n.Delta,
		Hypothesis: n.Hypothesis, Evidence: n.Evidence, Action: n.Action, DeclineConsequence: n.DeclineConsequence,
	}
}
