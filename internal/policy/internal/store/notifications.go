package store

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"time"
)

func (tx *Tx) AppendApprovalWithdrawn(ctx context.Context, row domain.IntentRecord, approvalID, reason string, now time.Time) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "approval.withdrawn:"+approvalID, row.TenantID, notify.TypeApprovalWithdrawn, "approval/"+approvalID, row.SituationID, map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "reason": reason, "source_authority": notify.SourceForTenant(row.TenantID),
	}, now, traceContext(row)); err != nil {
		return fmt.Errorf("append approval withdrawn notification: %w", err)
	}
	return nil
}

func (tx *Tx) AppendApprovalResolved(ctx context.Context, row domain.IntentRecord, approvalID, status, reason string, now time.Time) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx.tx, "approval.resolved:"+approvalID+":"+status, row.TenantID, notify.TypeApprovalResolved, "approval/"+approvalID, row.SituationID, map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"situation_id": row.SituationID, "situation_version": row.SituationVersion,
		"status": status, "reason": reason, "source_authority": notify.SourceForTenant(row.TenantID),
	}, now, traceContext(row)); err != nil {
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
