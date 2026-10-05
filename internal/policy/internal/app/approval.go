package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

type approvalAttempt struct {
	evaluation evaluation
	approval   domain.ApprovalRecord
	request    domain.ApprovalResolution
}

// ResolveApproval records the human decision and re-evaluates before publication.
func (g *Service) ResolveApproval(ctx context.Context, tx *store.Tx, r domain.ApprovalResolution) (domain.Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return domain.Result{ApprovalID: r.ID}, err
	}
	approval, err := tx.LoadApproval(ctx, r.ID, r.TenantID)
	if err != nil {
		return domain.Result{ApprovalID: r.ID}, err
	}
	row, err := tx.LoadIntent(ctx, approval.IntentID)
	if err != nil {
		return domain.Result{IntentID: approval.IntentID, ApprovalID: r.ID}, err
	}
	e := newEvaluation(row, r.Now)
	e.result.ApprovalID = r.ID
	return g.resolvePendingApproval(ctx, tx, approvalAttempt{evaluation: e, approval: approval, request: r})
}
func (g *Service) resolvePendingApproval(ctx context.Context, tx *store.Tx, a approvalAttempt) (domain.Result, error) {
	switch domain.ApprovalDisposition(a.evaluation.row, a.approval, a.request) {
	case "resolved":
		return g.audit(ctx, tx, a.evaluation, domain.Outcome{Status: a.approval.Status, Reason: "approval_already_resolved"})
	case "stale":
		return g.withdrawStaleApproval(ctx, tx, a)
	case "expired":
		return g.expireApproval(ctx, tx, a)
	default:
		return g.resolveAuthorizedApproval(ctx, tx, a)
	}
}
func (g *Service) withdrawStaleApproval(ctx context.Context, tx *store.Tx, a approvalAttempt) (domain.Result, error) {
	if err := tx.WithdrawApproval(ctx, a.request.ID, a.request.Now); err != nil {
		return a.evaluation.result, err
	}
	event := domain.ApprovalEvent{Intent: a.evaluation.row, ID: a.request.ID, Status: "denied", Reason: "situation_version_conflict", Now: a.request.Now}
	if err := tx.AppendApprovalWithdrawn(ctx, event); err != nil {
		return a.evaluation.result, err
	}
	if err := tx.AppendApprovalResolved(ctx, event); err != nil {
		return a.evaluation.result, err
	}
	return g.finish(ctx, tx, a.evaluation, domain.Outcome{Status: "denied", Reason: "situation_version_conflict"})
}
func (g *Service) expireApproval(ctx context.Context, tx *store.Tx, a approvalAttempt) (domain.Result, error) {
	if err := tx.ExpireApproval(ctx, a.request.ID, a.request.Now); err != nil {
		return a.evaluation.result, err
	}
	event := domain.ApprovalEvent{Intent: a.evaluation.row, ID: a.request.ID, Status: "expired", Reason: "approval_expired", Now: a.request.Now}
	if err := tx.AppendApprovalResolved(ctx, event); err != nil {
		return a.evaluation.result, err
	}
	return g.finish(ctx, tx, a.evaluation, domain.Outcome{Status: "expired", Reason: "approval_expired"})
}
func (g *Service) resolveAuthorizedApproval(ctx context.Context, tx *store.Tx, a approvalAttempt) (domain.Result, error) {
	if err := g.authorizeApproval(ctx, tx, a.evaluation.row, a.request); err != nil {
		return a.evaluation.result, fmt.Errorf("%w: %w", domain.ErrApprovalUnauthorized, err)
	}
	if err := recordApprovalResolution(ctx, tx, a); err != nil {
		return a.evaluation.result, err
	}
	if !a.request.Approved {
		return g.audit(ctx, tx, a.evaluation, domain.Outcome{Status: "denied", Reason: "approval_denied"})
	}
	return g.EvaluateIntent(ctx, tx, domain.EvaluationRequest{IntentID: a.evaluation.row.IntentID, Now: a.request.Now})
}
func recordApprovalResolution(ctx context.Context, tx *store.Tx, a approvalAttempt) error {
	status, policyStatus := domain.ApprovalDecision(a.request.Approved)
	if err := tx.ResolveApproval(ctx, a.request, status, "resolve approval "+a.request.ID); err != nil {
		return err
	}
	change := domain.IntentStatusChange{IntentID: a.evaluation.row.IntentID, Status: policyStatus, Now: a.request.Now, Operation: store.ResolveIntentStatus}
	if err := tx.SetIntentStatus(ctx, change); err != nil {
		return err
	}
	return tx.AppendApprovalResolved(ctx, domain.ApprovalEvent{Intent: a.evaluation.row, ID: a.request.ID, Status: status, Reason: a.request.Reason, Now: a.request.Now})
}
