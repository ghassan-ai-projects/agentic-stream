package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

// EvaluateIntent runs the ordered policy gates and atomically creates either
// an approval request or a Command plus outbox row.
func (g *Gateway) EvaluateIntent(ctx context.Context, tx *sql.Tx, intentID string, now time.Time) (Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return Result{IntentID: intentID}, err
	}
	row, err := g.loadIntent(ctx, tx, intentID)
	if err != nil {
		return Result{IntentID: intentID}, err
	}
	if err := g.assertPolicyEpoch(ctx, tx, row); err != nil {
		return g.denyEpochIntent(ctx, tx, row, err, now)
	}
	result := Result{IntentID: row.IntentID, DecisionID: row.DecisionID}
	if row.PolicyStatus != "pending" {
		return g.evaluateExisting(ctx, tx, row, result, now)
	}
	return g.evaluatePending(ctx, tx, row, result, now)
}

func (g *Gateway) assertPolicyEpoch(ctx context.Context, tx *sql.Tx, row intentRow) error {
	if g.epochControl == nil {
		return nil
	}
	if err := g.epochControl.AssertDecisionTx(ctx, tx, row.PolicyEpoch); err != nil {
		return fmt.Errorf("assert policy epoch: %w", err)
	}
	return nil
}

func (g *Gateway) evaluateExisting(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	if row.PolicyStatus == "approval_required" {
		if result, expired, err := g.expireExistingApproval(ctx, tx, row, result, now); expired || err != nil {
			return result, err
		}
	}
	result.Result = row.PolicyStatus
	result.Reason = "already_evaluated"
	if err := bindExistingResult(ctx, tx, row, &result); err != nil {
		return result, err
	}
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func (g *Gateway) expireExistingApproval(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, bool, error) {
	var approvalID, expiry string
	if err := tx.QueryRowContext(ctx, "SELECT approval_id, expires_at FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&approvalID, &expiry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, false, nil
		}
		return result, false, fmt.Errorf("load pending approval for expiry: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
	if err == nil && expiresAt.After(now) {
		return result, false, nil
	}
	return g.recordExpiredApproval(ctx, tx, row, approvalID, result, now)
}

func (g *Gateway) markStale(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'stale', updated_at = ? WHERE intent_id = ?", domain.FormatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("mark stale intent: %w", err)
	}
	return g.audit(ctx, tx, row, result, "stale", "situation_version_stale", now)
}

func (g *Gateway) routeIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
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

func (g *Gateway) routeConsequentialIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	if g.calibration != nil && row.SituationType != "" && row.ExecutorVersion != "" {
		if err := g.calibration.AssertCalibration(ctx, tx, qualification.CalibrationArtifact{
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
func (g *Gateway) approveOrRequireApproval(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	var approvedApproval string
	err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'approved' ORDER BY decided_at DESC LIMIT 1", row.IntentID).Scan(&approvedApproval)
	switch {
	case err == nil:
		result.ApprovalID = approvedApproval
		result.Reason = "approved_by_human"
		return g.approveAutomatic(ctx, tx, row, intent, result, now)
	case errors.Is(err, sql.ErrNoRows):
		return g.requireApproval(ctx, tx, row, intent, result, expiresAt, now)
	default:
		return result, fmt.Errorf("load approved approval: %w", err)
	}
}

func (g *Gateway) denyEpochIntent(ctx context.Context, tx *sql.Tx, row intentRow, err error, now time.Time) (Result, error) {
	reason := "epoch_killed"
	if errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		reason = "epoch_unbound"
	}
	return g.finish(ctx, tx, row, Result{IntentID: row.IntentID, DecisionID: row.DecisionID}, "denied", reason, now)
}

func bindExistingResult(ctx context.Context, tx *sql.Tx, row intentRow, result *Result) error {
	if row.PolicyStatus == "approved" {
		if err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&result.CommandID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load existing command id: %w", err)
		}
	}
	if row.PolicyStatus == "approval_required" {
		if err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&result.ApprovalID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load pending approval id: %w", err)
		}
	}
	return nil
}

func (g *Gateway) recordExpiredApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, result Result, now time.Time) (Result, bool, error) {
	if err := approvalledger.ExpireIntent(ctx, tx, row.IntentID); err != nil {
		return result, true, fmt.Errorf("expire approval: %w", err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, true, err
	}
	result, err := g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
	return result, true, err
}
