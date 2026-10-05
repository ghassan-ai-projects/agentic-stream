package app

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"time"
)

func (g *Service) ResolveApproval(ctx context.Context, tx *store.Tx, request domain.ApprovalResolution) (domain.Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return domain.Result{ApprovalID: request.ID}, err
	}
	approval, err := tx.LoadApproval(ctx, request.ID)
	if err != nil {
		return domain.Result{ApprovalID: request.ID}, err
	}
	row, err := tx.LoadIntent(ctx, approval.IntentID)
	if err != nil {
		return domain.Result{IntentID: approval.IntentID, ApprovalID: request.ID}, err
	}
	result := domain.Result{IntentID: approval.IntentID, DecisionID: row.DecisionID, ApprovalID: request.ID}
	return g.resolvePendingApproval(ctx, tx, row, approval, request, result)
}

func (g *Service) resolvePendingApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approval domain.ApprovalRecord, request domain.ApprovalResolution, result domain.Result) (domain.Result, error) {
	if approval.Status != "pending" {
		result.Result, result.Reason = approval.Status, "approval_already_resolved"
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, request.Now)
	}
	if request.Approved && row.CurrentSituation != row.SituationVersion {
		return g.withdrawStaleApproval(ctx, tx, row, request.ID, result, request.Now)
	}
	if domain.ApprovalExpired(approval.ExpiresAt, request.Now) {
		return g.expireApproval(ctx, tx, row, request.ID, result, request.Now)
	}
	return g.resolveAuthorizedApproval(ctx, tx, row, request, result)
}

func (g *Service) withdrawStaleApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID string, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.WithdrawApproval(ctx, approvalID, now); err != nil {
		return result, err
	}
	if err := tx.AppendApprovalWithdrawn(ctx, row, approvalID, "situation_version_conflict", now); err != nil {
		return result, err
	}
	if err := tx.AppendApprovalResolved(ctx, row, approvalID, "denied", "situation_version_conflict", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "denied", "situation_version_conflict", now)
}

func (g *Service) expireApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID string, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.ExpireApproval(ctx, approvalID, now); err != nil {
		return result, err
	}
	if err := tx.AppendApprovalResolved(ctx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
}

func (g *Service) resolveAuthorizedApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, request domain.ApprovalResolution, result domain.Result) (domain.Result, error) {
	if request.Approved {
		if err := g.authorizeApproval(ctx, tx, row, request.ID, request.Approver, request.Relay, request.Signature); err != nil {
			return g.denyUnauthorizedApproval(ctx, tx, row, request.ID, request.Approver, request.Relay, err, result, request.Now)
		}
	}
	if err := recordApprovalResolution(ctx, tx, row, request); err != nil {
		return result, err
	}
	if !request.Approved {
		return g.audit(ctx, tx, row, result, "denied", "approval_denied", request.Now)
	}
	return g.EvaluateIntent(ctx, tx, domain.EvaluationRequest{IntentID: row.IntentID, Now: request.Now})
}

func (g *Service) denyUnauthorizedApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID, approver, relay string, authErr error, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.ResolveApproval(ctx, domain.ApprovalResolution{ID: approvalID, Approver: approver, Relay: relay, Reason: authErr.Error(), Now: now}, "denied"); err != nil {
		return result, err
	}
	if err := tx.AppendApprovalResolved(ctx, row, approvalID, "denied", "approval_principal_not_authorized", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "denied", "approval_principal_not_authorized", now)
}

func recordApprovalResolution(ctx context.Context, tx *store.Tx, row domain.IntentRecord, request domain.ApprovalResolution) error {
	status, policyStatus := domain.ApprovalDecision(request.Approved)
	if err := tx.ResolveApproval(ctx, request, status); err != nil {
		return err
	}
	if err := tx.SetIntentStatus(ctx, row.IntentID, policyStatus, request.Now, store.ResolveIntentStatus); err != nil {
		return err
	}
	return tx.AppendApprovalResolved(ctx, row, request.ID, status, request.Reason, request.Now)
}
