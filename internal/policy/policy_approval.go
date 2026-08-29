package policy

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

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
	result := Result{IntentID: approval.intentID, DecisionID: row.DecisionID, ApprovalID: approvalID}
	if approval.status != "pending" {
		result.Result = approval.status
		result.Reason = "approval_already_resolved"
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}
	if approved && row.CurrentSituation != row.SituationVersion {
		return g.withdrawStaleApproval(ctx, tx, row, approvalID, result, now)
	}
	if approvalExpired(approval.expiresAt, now) {
		return g.expireApproval(ctx, tx, row, approvalID, result, now)
	}
	if approved {
		if err := g.authorizeApproval(ctx, tx, row, approvalID, approver, relay, signature); err != nil {
			return g.denyUnauthorizedApproval(ctx, tx, row, approvalID, approver, relay, err, result, now)
		}
	}
	status, policyStatus := approvalDecision(approved)
	if err := updateApproval(ctx, tx, approvalID, status, approver, relay, reason, now); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", policyStatus, formatTime(now), approval.intentID); err != nil {
		return result, fmt.Errorf("update approved intent %s: %w", approval.intentID, err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, status, reason, now); err != nil {
		return result, err
	}
	if !approved {
		return g.audit(ctx, tx, row, result, "denied", "approval_denied", now)
	}
	return g.EvaluateIntent(ctx, tx, approval.intentID, now)
}

type approvalRow struct {
	intentID  string
	status    string
	expiresAt string
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

func approvalExpired(expiresAt string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	return err != nil || !expires.After(now)
}

func (g *Gateway) withdrawStaleApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, result Result, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE approvals
		SET status = 'denied', decided_at = ?, withdrawn_at = ?, withdrawal_reason = ?, reason = ?
		WHERE approval_id = ? AND status = 'pending'`, formatTime(now), formatTime(now), "situation_version_conflict", "approval_withdrawn", approvalID); err != nil {
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
	if _, err := tx.ExecContext(ctx, "UPDATE approvals SET status = 'expired', decided_at = ?, reason = ? WHERE approval_id = ? AND status = 'pending'", formatTime(now), "approval_expired", approvalID); err != nil {
		return result, fmt.Errorf("expire approval %s: %w", approvalID, err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
}

func approvalDecision(approved bool) (status, policyStatus string) {
	if approved {
		return "approved", "pending"
	}
	return "denied", "denied"
}

func updateApproval(ctx context.Context, tx *sql.Tx, approvalID, status, approver, relay, reason string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approvals SET status = ?, decided_at = ?, approver_identity = ?, relay_identity = ?, reason = ?
		WHERE approval_id = ? AND status = 'pending'`, status, formatTime(now), approver, relay, reason, approvalID); err != nil {
		return fmt.Errorf("resolve approval %s: %w", approvalID, err)
	}
	return nil
}

func (g *Gateway) denyUnauthorizedApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string, authErr error, result Result, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approvals SET status = 'denied', decided_at = ?, approver_identity = ?, relay_identity = ?, reason = ?
		WHERE approval_id = ? AND status = 'pending'`, formatTime(now), approver, relay, authErr.Error(), approvalID); err != nil {
		return result, fmt.Errorf("record unauthorized approval: %w", err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "denied", "approval_principal_not_authorized", now); err != nil {
		return result, err
	}
	return g.finish(ctx, tx, row, result, "denied", "approval_principal_not_authorized", now)
}

func (g *Gateway) authorizeApproval(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, approver, relay string, signature []byte) error {
	if approver == "" || relay == "" || approver == relay {
		return fmt.Errorf("relay and approver must be distinct registered principals")
	}
	var entityID string
	if err := tx.QueryRowContext(ctx, "SELECT entity_id FROM situations WHERE situation_id = ?", row.SituationID).Scan(&entityID); err != nil {
		return fmt.Errorf("load approval entity: %w", err)
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", relay, row.TenantID).Scan(&active); err != nil || active != 1 {
		return fmt.Errorf("relay principal is not active")
	}
	var publicKey []byte
	if err := tx.QueryRowContext(ctx, "SELECT public_key FROM principals WHERE principal_id = ? AND tenant_id = ? AND status = 'active'", approver, row.TenantID).Scan(&publicKey); err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("approver principal has no valid verification key")
	}
	var expiresAt, nonce string
	if err := tx.QueryRowContext(ctx, "SELECT expires_at, nonce FROM approvals WHERE approval_id = ? AND status = 'pending'", approvalID).Scan(&expiresAt, &nonce); err != nil {
		return fmt.Errorf("load approval assertion binding: %w", err)
	}
	assertion, err := ApprovalAssertionSigningBytes(ApprovalAssertion{
		ApprovalID: approvalID, IntentID: row.IntentID, DecisionID: row.DecisionID, TenantID: row.TenantID,
		SituationID: row.SituationID, SituationVersion: row.SituationVersion, RiskClass: row.RiskClass,
		IntentDigest: "sha256:" + hex.EncodeToString(row.IntentSHA), DecisionDigest: "sha256:" + hex.EncodeToString(row.DecisionSHA),
		ExpiresAt: expiresAt, Nonce: nonce, ApproverID: approver, RelayID: relay,
	})
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), assertion, signature) {
		return fmt.Errorf("approval assertion signature is invalid")
	}
	assertionDigest := sha256.Sum256(assertion)
	if _, err := tx.ExecContext(ctx, "UPDATE approvals SET assertion_sha256 = ?, nonce = nonce WHERE approval_id = ? AND status = 'pending'", assertionDigest[:], approvalID); err != nil {
		return fmt.Errorf("record approval assertion: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM principals p
		JOIN principal_roles pr ON pr.principal_id = p.principal_id
		JOIN approval_authorities aa ON aa.role_id = pr.role_id
		WHERE p.principal_id = ? AND p.tenant_id = ? AND p.status = 'active'
		  AND aa.tenant_id = ? AND aa.entity_id = ? AND aa.risk_class = ?`,
		approver, row.TenantID, row.TenantID, entityID, row.RiskClass).Scan(&active); err != nil || active == 0 {
		return fmt.Errorf("approver principal lacks authority")
	}
	return nil
}
