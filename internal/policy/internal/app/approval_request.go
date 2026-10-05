package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"time"
)

func (g *Service) requireApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, expiresAt, now time.Time) (domain.Result, error) {
	if approvalID, err := tx.PendingApproval(ctx, row.IntentID); err != nil {
		return result, err
	} else if approvalID != "" {
		result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}
	return g.openApproval(ctx, tx, row, intent, result, expiresAt, now)
}

func (g *Service) openApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, expiresAt, now time.Time) (domain.Result, error) {
	request, err := g.prepareApprovalRequest(ctx, tx, row, intent, expiresAt)
	if err != nil {
		return result, err
	}
	if err := tx.RequestApproval(ctx, row.IntentID, request, expiresAt, now); err != nil {
		return result, err
	}
	return g.publishApprovalRequest(ctx, tx, row, request, result, now)
}

func (g *Service) prepareApprovalRequest(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, expiresAt time.Time) (domain.ApprovalRequest, error) {
	request := domain.ApprovalRequest{ID: g.idGen.New(ids.PrefixApproval)}
	nonceDigest := sha256.Sum256([]byte(request.ID + "|" + row.IntentID))
	request.Nonce = hex.EncodeToString(nonceDigest[:])
	var err error
	request.Data, err = approvalNotificationData(ctx, tx, row, request.ID, expiresAt, intent)
	if err != nil {
		return request, fmt.Errorf("build approval notification: %w", err)
	}
	request.JSON, err = domain.ApprovalRequestJSON(row, intent, request)
	return request, err
}

func (g *Service) publishApprovalRequest(ctx context.Context, tx *store.Tx, row domain.IntentRecord, request domain.ApprovalRequest, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, row.IntentID, "approval_required", now, store.RequireApprovalStatus); err != nil {
		return result, err
	}
	if err := tx.AppendApprovalRequested(ctx, row, request, now); err != nil {
		return result, err
	}
	result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", request.ID
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}
