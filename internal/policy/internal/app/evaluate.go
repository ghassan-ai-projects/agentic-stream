package app

import (
	"context"
	"errors"
	"fmt"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"time"
)

func (g *Service) EvaluateIntent(ctx context.Context, tx *store.Tx, request domain.EvaluationRequest) (domain.Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return domain.Result{IntentID: request.IntentID}, err
	}
	row, err := tx.LoadIntent(ctx, request.IntentID)
	if err != nil {
		return domain.Result{IntentID: request.IntentID}, err
	}
	if err := g.assertPolicyEpoch(ctx, tx, row); err != nil {
		return g.denyEpochIntent(ctx, tx, row, err, request.Now)
	}
	result := domain.Result{IntentID: row.IntentID, DecisionID: row.DecisionID}
	if row.PolicyStatus != "pending" {
		return g.evaluateExisting(ctx, tx, row, result, request.Now)
	}
	return g.evaluatePending(ctx, tx, row, result, request.Now)
}

func (g *Service) assertPolicyEpoch(ctx context.Context, tx *store.Tx, row domain.IntentRecord) error {
	if err := tx.Assert(ctx, g.fences.DecisionEpoch, row.PolicyEpoch); err != nil {
		return fmt.Errorf("assert policy epoch: %w", err)
	}
	return nil
}

func (g *Service) evaluateExisting(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, now time.Time) (domain.Result, error) {
	if row.PolicyStatus == "approval_required" {
		if result, expired, err := g.expireExistingApproval(ctx, tx, row, result, now); expired || err != nil {
			return result, err
		}
	}
	result.Result = row.PolicyStatus
	result.Reason = "already_evaluated"
	if err := tx.BindExistingResult(ctx, row, &result); err != nil {
		return result, err
	}
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func (g *Service) expireExistingApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, now time.Time) (domain.Result, bool, error) {
	approvalID, expiry, err := tx.PendingApprovalExpiry(ctx, row.IntentID)
	if err != nil || approvalID == "" {
		return result, false, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
	if err == nil && expiresAt.After(now) {
		return result, false, nil
	}
	return g.recordExpiredApproval(ctx, tx, row, approvalID, result, now)
}

func (g *Service) markStale(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, row.IntentID, "stale", now, store.MarkStaleStatus); err != nil {
		return result, err
	}
	return g.audit(ctx, tx, row, result, "stale", "situation_version_stale", now)
}

func (g *Service) routeIntent(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, expiresAt, now time.Time) (domain.Result, error) {
	// The risk-class policy document is authoritative and enforced first: R3/R4
	// are unconditionally denied regardless of a catalog requires_approval flag.
	// requires_approval is an override only for the auto-approvable R0/R1 tier,
	// and it consults an already-approved approval before creating a new request
	// so a resolved approval terminates in dispatch instead of spawning another
	// approval on the next EvaluateIntent (the approve -> re-pending loop).
	switch row.RiskClass {
	case "R0", "R1":
		if row.RequiresApproval != 0 {
			return g.approveOrRequireApproval(ctx, tx, row, intent, result, expiresAt, now)
		}
		return g.approveAutomatic(ctx, tx, row, intent, result, now)
	case "R2":
		return g.routeConsequentialIntent(ctx, tx, row, intent, result, expiresAt, now)
	case "R3", "R4":
		return g.finish(ctx, tx, row, result, "denied", "risk_policy_denied", now)
	default:
		return g.finish(ctx, tx, row, result, "denied", "unknown_risk_class", now)
	}
}

func (g *Service) routeConsequentialIntent(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, expiresAt, now time.Time) (domain.Result, error) {
	if g.calibration != nil && row.SituationType != "" && row.ExecutorVersion != "" {
		if err := tx.AssertCalibration(ctx, g.calibration, qualification.CalibrationArtifact{
			Domain: row.SituationType, ModelRevision: row.ExecutorVersion,
		}); err == nil {
			return g.approveAutomatic(ctx, tx, row, intent, result.WithReason("calibrated_automation"), now)
		}
	}
	return g.approveOrRequireApproval(ctx, tx, row, intent, result, expiresAt, now)
}

// approveOrRequireApproval dispatches when a human has already approved this
// intent, otherwise it opens a fresh approval request. Consulting the existing
// approval is what breaks the approve -> re-pending -> new-approval loop: once
// ResolveApproval marks the approval 'approved' and re-runs EvaluateIntent,
// this path finds that row and terminates in dispatch.
func (g *Service) approveOrRequireApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, expiresAt, now time.Time) (domain.Result, error) {
	approvedApproval, err := tx.ApprovedApproval(ctx, row.IntentID)
	switch {
	case err == nil && approvedApproval != "":
		result.ApprovalID = approvedApproval
		result.Reason = "approved_by_human"
		return g.approveAutomatic(ctx, tx, row, intent, result, now)
	case err == nil:
		return g.requireApproval(ctx, tx, row, intent, result, expiresAt, now)
	default:
		return result, err
	}
}

func (g *Service) denyEpochIntent(ctx context.Context, tx *store.Tx, row domain.IntentRecord, err error, now time.Time) (domain.Result, error) {
	reason := "epoch_killed"
	if errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		reason = "epoch_unbound"
	}
	return g.finish(ctx, tx, row, domain.Result{IntentID: row.IntentID, DecisionID: row.DecisionID}, "denied", reason, now)
}

func (g *Service) recordExpiredApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approvalID string, result domain.Result, now time.Time) (domain.Result, bool, error) {
	if err := tx.ExpireIntentApproval(ctx, row.IntentID); err != nil {
		return result, true, err
	}
	if err := tx.AppendApprovalResolved(ctx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, true, err
	}
	result, err := g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
	return result, true, err
}
