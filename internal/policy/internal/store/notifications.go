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
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "approval.withdrawn:"+event.ID, event.Intent.TenantID, notify.TypeApprovalWithdrawn, "approval/"+event.ID, event.Intent.SituationID, map[string]any{
		"tenant_id": event.Intent.TenantID, "approval_id": event.ID, "intent_id": event.Intent.IntentID, "situation_id": event.Intent.SituationID,
		"situation_version": event.Intent.SituationVersion, "reason": event.Reason, "source_authority": notify.SourceForTenant(event.Intent.TenantID),
	}, event.Now, traceContext(event.Intent)); err != nil {
		return fmt.Errorf("append approval withdrawn notification: %w", err)
	}
	return nil
}

// AppendApprovalResolved publishes a durable approval disposition.
func (tx *Tx) AppendApprovalResolved(ctx context.Context, event domain.ApprovalEvent) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "approval.resolved:"+event.ID+":"+event.Status, event.Intent.TenantID, notify.TypeApprovalResolved, "approval/"+event.ID, event.Intent.SituationID, map[string]any{
		"tenant_id": event.Intent.TenantID, "approval_id": event.ID, "intent_id": event.Intent.IntentID, "decision_id": event.Intent.DecisionID,
		"situation_id": event.Intent.SituationID, "situation_version": event.Intent.SituationVersion,
		"status": event.Status, "reason": event.Reason, "source_authority": notify.SourceForTenant(event.Intent.TenantID),
	}, event.Now, traceContext(event.Intent)); err != nil {
		return fmt.Errorf("append approval resolved notification: %w", err)
	}
	return nil
}

func traceContext(row domain.IntentRecord) contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: row.Traceparent, Tracestate: row.Tracestate}
}

// NotificationSource preserves the shared tenant lifecycle-event source.
func (tx *Tx) NotificationSource(tenant string) string { return notify.SourceForTenant(tenant) }

// AppendApprovalRequested publishes the sealed request notification.
func (tx *Tx) AppendApprovalRequested(ctx context.Context, row domain.IntentRecord, request domain.ApprovalRequest, now time.Time) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "approval.requested:"+request.ID, row.TenantID, notify.TypeApprovalRequested, "approval/"+request.ID, row.SituationID, request.Data, now, traceContext(row)); err != nil {
		return fmt.Errorf("append approval requested notification: %w", err)
	}
	return nil
}
