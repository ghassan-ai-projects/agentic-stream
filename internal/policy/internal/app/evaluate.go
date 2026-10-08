package app

import (
	"context"
	"errors"
	"fmt"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) EvaluateIntent(ctx context.Context, tx *store.Tx, request domain.EvaluationRequest) (domain.Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return domain.Result{IntentID: request.IntentID}, err
	}
	row, err := tx.LoadIntent(ctx, request.IntentID)
	if err != nil {
		return domain.Result{IntentID: request.IntentID}, err
	}
	return g.evaluateLoaded(ctx, tx, newEvaluation(row, request.Now))
}

func (g *Service) evaluateLoaded(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	if err := g.assertPolicyEpoch(ctx, tx, e.row); err != nil {
		return g.denyEpochIntent(ctx, tx, e, err)
	}
	if e.row.PolicyStatus != "pending" {
		return g.evaluateExisting(ctx, tx, e)
	}
	return g.evaluatePending(ctx, tx, e)
}

func (g *Service) assertPolicyEpoch(ctx context.Context, tx *store.Tx, row domain.IntentRecord) error {
	if err := tx.Assert(ctx, g.fences.DecisionEpoch, row.PolicyEpoch); err != nil {
		return fmt.Errorf("assert policy epoch: %w", err)
	}
	return nil
}

func (g *Service) evaluateExisting(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	if e.row.PolicyStatus == "approval_required" {
		if result, expired, err := g.expireExistingApproval(ctx, tx, e); expired || err != nil {
			return result, err
		}
	}
	e.result.Result = e.row.PolicyStatus
	e.result.Reason = "already_evaluated"
	if err := tx.BindExistingResult(ctx, e.row, &e.result); err != nil {
		return e.result, err
	}
	return g.audit(ctx, tx, e, domain.Outcome{Status: e.result.Result, Reason: e.result.Reason})
}

func (g *Service) expireExistingApproval(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, bool, error) {
	approvalID, expiry, err := tx.PendingApprovalExpiry(ctx, e.row.IntentID)
	if err != nil || approvalID == "" {
		return e.result, false, err
	}
	if expiry.After(e.now) {
		return e.result, false, nil
	}
	return g.recordExpiredApproval(ctx, tx, e, approvalID)
}

func (g *Service) markStale(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: e.row.IntentID, Status: "stale", Now: e.now, Operation: store.MarkStaleStatus}); err != nil {
		return e.result, err
	}
	return g.audit(ctx, tx, e, domain.Outcome{Status: "stale", Reason: "situation_version_stale"})
}

func (g *Service) routeIntent(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	route, reason := domain.RiskRoute(e.row)
	switch route {
	case "automatic":
		return g.approveAutomatic(ctx, tx, e)
	case "approval":
		return g.approveOrRequireApproval(ctx, tx, e)
	default:
		return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: reason})
	}
}

func (g *Service) approveOrRequireApproval(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	approvedApproval, err := tx.ApprovedApproval(ctx, e.row.IntentID)
	switch {
	case err == nil && approvedApproval != "":
		e.result.ApprovalID = approvedApproval
		e.result.Reason = "approved_by_human"
		return g.approveAutomatic(ctx, tx, e)
	case err == nil:
		return g.requireApproval(ctx, tx, e)
	default:
		return e.result, err
	}
}

func (g *Service) denyEpochIntent(ctx context.Context, tx *store.Tx, e evaluation, err error) (domain.Result, error) {
	reason := "epoch_killed"
	if errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		reason = "epoch_unbound"
	}
	return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: reason})
}

func (g *Service) recordExpiredApproval(ctx context.Context, tx *store.Tx, e evaluation, approvalID string) (domain.Result, bool, error) {
	if err := tx.ExpireIntentApproval(ctx, e.row.IntentID); err != nil {
		return e.result, true, err
	}
	if err := tx.AppendApprovalResolved(ctx, domain.ApprovalEvent{Intent: e.row, ID: approvalID, Status: "expired", Reason: "approval_expired", Now: e.now}); err != nil {
		return e.result, true, err
	}
	result, err := g.finish(ctx, tx, e, domain.Outcome{Status: "expired", Reason: "approval_expired"})
	return result, true, err
}
