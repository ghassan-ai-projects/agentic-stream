package policy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

type approvalRow struct {
	intentID  string
	status    string
	expiresAt string
}

type approvalResolution struct {
	id              string
	approved        bool
	approver, relay string
	signature       []byte
	reason          string
	now             time.Time
}

// ResolveApproval records a human approval decision and, when approved,
// immediately re-runs the full policy gate before creating a Command.
func (g *Gateway) ResolveApproval(ctx context.Context, tx *sql.Tx, approvalID string, approved bool, approver, relay string, signature []byte, reason string, now time.Time) (Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return Result{ApprovalID: approvalID}, err
	}
	approval, err := loadApproval(ctx, tx, approvalID)
	if err != nil {
		return Result{ApprovalID: approvalID}, err
	}
	row, err := g.loadIntent(ctx, tx, approval.intentID)
	if err != nil {
		return Result{IntentID: approval.intentID, ApprovalID: approvalID}, err
	}
	request := approvalResolution{approvalID, approved, approver, relay, signature, reason, now}
	result := Result{IntentID: approval.intentID, DecisionID: row.DecisionID, ApprovalID: approvalID}
	return g.resolvePendingApproval(ctx, tx, row, approval, request, result)
}

func loadApproval(ctx context.Context, tx *sql.Tx, approvalID string) (approvalRow, error) {
	var approval approvalRow
	err := tx.QueryRowContext(ctx, "SELECT intent_id, status, expires_at FROM approvals WHERE approval_id = ?", approvalID).
		Scan(&approval.intentID, &approval.status, &approval.expiresAt)
	if err != nil {
		return approval, fmt.Errorf("load approval %s: %w", approvalID, err)
	}
	return approval, nil
}

func (g *Gateway) resolvePendingApproval(ctx context.Context, tx *sql.Tx, row intentRow, approval approvalRow, request approvalResolution, result Result) (Result, error) {
	if approval.status != "pending" {
		result.Result, result.Reason = approval.status, "approval_already_resolved"
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, request.now)
	}
	if request.approved && row.CurrentSituation != row.SituationVersion {
		return g.withdrawStaleApproval(ctx, tx, row, request.id, result, request.now)
	}
	if domain.ApprovalExpired(approval.expiresAt, request.now) {
		return g.expireApproval(ctx, tx, row, request.id, result, request.now)
	}
	return g.resolveAuthorizedApproval(ctx, tx, row, request, result)
}

func (g *Gateway) withdrawStaleApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, result Result, now time.Time) (Result, error) {
	if err := approvalledger.Withdraw(ctx, tx, approvalID, domain.FormatTime(now)); err != nil {
		return result, fmt.Errorf("withdraw stale approval: %w", err)
	}
	if err := appendApprovalWithdrawn(ctx, tx, row, approvalID, "situation_version_conflict", now); err != nil {
		return result, err
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "denied", "situation_version_conflict", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "denied", "situation_version_conflict", now)
}

func (g *Gateway) expireApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, result Result, now time.Time) (Result, error) {
	if err := approvalledger.Expire(ctx, tx, approvalID, domain.FormatTime(now)); err != nil {
		return result, fmt.Errorf("expire approval %s: %w", approvalID, err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
}

func (g *Gateway) resolveAuthorizedApproval(ctx context.Context, tx *sql.Tx, row intentRow, request approvalResolution, result Result) (Result, error) {
	if request.approved {
		if err := g.authorizeApproval(ctx, tx, row, request.id, request.approver, request.relay, request.signature); err != nil {
			return g.denyUnauthorizedApproval(ctx, tx, row, request.id, request.approver, request.relay, err, result, request.now)
		}
	}
	if err := recordApprovalResolution(ctx, tx, row, request); err != nil {
		return result, err
	}
	if !request.approved {
		return g.audit(ctx, tx, row, result, "denied", "approval_denied", request.now)
	}
	return g.EvaluateIntent(ctx, tx, row.IntentID, request.now)
}

func (g *Gateway) denyUnauthorizedApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string, authErr error, result Result, now time.Time) (Result, error) {
	if err := approvalledger.Resolve(ctx, tx, approvalID, "denied", approver, relay, authErr.Error(), domain.FormatTime(now)); err != nil {
		return result, fmt.Errorf("record unauthorized approval: %w", err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "denied", "approval_principal_not_authorized", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "denied", "approval_principal_not_authorized", now)
}

func recordApprovalResolution(ctx context.Context, tx *sql.Tx, row intentRow, request approvalResolution) error {
	status, policyStatus := domain.ApprovalDecision(request.approved)
	if err := approvalledger.Resolve(ctx, tx, request.id, status, request.approver, request.relay, request.reason, domain.FormatTime(request.now)); err != nil {
		return fmt.Errorf("resolve approval %s: %w", request.id, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", policyStatus, domain.FormatTime(request.now), row.IntentID); err != nil {
		return fmt.Errorf("update approved intent %s: %w", row.IntentID, err)
	}
	return appendApprovalResolved(ctx, tx, row, request.id, status, request.reason, request.now)
}
