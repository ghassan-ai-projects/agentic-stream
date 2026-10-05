package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func approvalNotificationData(ctx context.Context, tx *store.Tx, e evaluation, approvalID string) (map[string]any, error) {
	if err := domain.CompleteDigest(e.row.IntentSHA, "intent"); err != nil {
		return nil, err
	}
	evidence, err := loadApprovalContext(ctx, tx, e)
	if err != nil {
		return nil, err
	}
	return domain.BuildApprovalNotification(domain.ApprovalNotice{Row: e.row, ID: approvalID, ExpiresAt: e.expiresAt, Intent: e.documents.Intent, Context: evidence}), nil
}
func loadApprovalContext(ctx context.Context, tx *store.Tx, e evaluation) (domain.ApprovalContext, error) {
	snapshot, err := tx.ApprovalSnapshotDigest(ctx, e.row)
	if err != nil {
		return domain.ApprovalContext{}, err
	}
	if err := domain.CompleteDigest(snapshot, "approval snapshot"); err != nil {
		return domain.ApprovalContext{}, err
	}
	delta, err := tx.ApprovalDelta(ctx, e.row.EpisodeID)
	if err != nil {
		return domain.ApprovalContext{}, err
	}
	return domain.ApprovalContext{Snapshot: snapshot, Delta: delta, Decision: e.documents.Decision, Source: tx.NotificationSource(e.row.TenantID)}, nil
}
