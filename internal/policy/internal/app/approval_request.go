package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) requireApproval(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	if approvalID, err := tx.PendingApproval(ctx, e.row.IntentID); err != nil {
		return e.result, err
	} else if approvalID != "" {
		e.result.Result, e.result.Reason, e.result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
		return g.audit(ctx, tx, e, domain.Outcome{Status: e.result.Result, Reason: e.result.Reason})
	}
	return g.openApproval(ctx, tx, e)
}

func (g *Service) openApproval(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	request, err := g.prepareApprovalRequest(ctx, tx, e)
	if err != nil {
		return e.result, err
	}
	if err := tx.RequestApproval(ctx, domain.ApprovalPublication{IntentID: e.row.IntentID, Request: request, ExpiresAt: e.expiresAt, Now: e.now}); err != nil {
		return e.result, err
	}
	return g.publishApprovalRequest(ctx, tx, e, request)
}

func (g *Service) prepareApprovalRequest(ctx context.Context, tx *store.Tx, e evaluation) (domain.ApprovalRequest, error) {
	request := domain.ApprovalRequest{ID: g.idGen.New(ids.PrefixApproval)}
	request.Nonce = domain.ApprovalNonce(request.ID, e.row.IntentID)
	var err error
	request.Data, err = approvalNotificationData(ctx, tx, e, request.ID)
	if err != nil {
		return request, fmt.Errorf("build approval notification: %w", err)
	}
	request.JSON, err = domain.ApprovalRequestJSON(e.row, e.documents.Intent, request)
	return request, err
}

func (g *Service) publishApprovalRequest(ctx context.Context, tx *store.Tx, e evaluation, request domain.ApprovalRequest) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: e.row.IntentID, Status: "approval_required", Now: e.now, Operation: store.RequireApprovalStatus}); err != nil {
		return e.result, err
	}
	if err := tx.AppendApprovalRequested(ctx, e.row, request, e.now); err != nil {
		return e.result, err
	}
	e.result.Result, e.result.Reason, e.result.ApprovalID = "approval_required", "risk_requires_approval", request.ID
	return g.audit(ctx, tx, e, domain.Outcome{Status: e.result.Result, Reason: e.result.Reason})
}
