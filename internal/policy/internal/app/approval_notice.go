package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"time"
)

func approvalNotificationData(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID string, expiresAt time.Time, intent map[string]any) (map[string]any, error) {
	if len(row.IntentSHA) != sha256.Size {
		return nil, fmt.Errorf("intent digest is incomplete")
	}
	context, err := loadApprovalContext(ctx, tx, row)
	if err != nil {
		return nil, err
	}
	return domain.BuildApprovalNotification(row, approvalID, expiresAt, intent, context), nil
}

func loadApprovalContext(ctx context.Context, tx *store.Tx, row domain.IntentRecord) (domain.ApprovalContext, error) {
	snapshotSHA, err := tx.ApprovalSnapshotDigest(ctx, row)
	if err != nil {
		return domain.ApprovalContext{}, err
	}
	delta, err := tx.ApprovalDelta(ctx, row.EpisodeID)
	if err != nil {
		return domain.ApprovalContext{}, err
	}
	decision, err := domain.DecodeApprovalDecision(row.DecisionJSON)
	if err != nil {
		return domain.ApprovalContext{}, err
	}
	return domain.ApprovalContext{Snapshot: snapshotSHA, Delta: delta, Decision: decision, Source: tx.NotificationSource(row.TenantID)}, nil
}
