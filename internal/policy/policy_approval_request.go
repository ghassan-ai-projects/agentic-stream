package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (g *Gateway) requireApproval(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	if approvalID, err := pendingApproval(ctx, tx, row.IntentID); err != nil {
		return result, err
	} else if approvalID != "" {
		result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}
	return g.openApproval(ctx, tx, row, intent, result, expiresAt, now)
}

type approvalRequestDocument struct {
	id, nonce string
	data      map[string]any
	json      []byte
}

func (g *Gateway) openApproval(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	request, err := g.prepareApprovalRequest(ctx, tx, row, intent, expiresAt)
	if err != nil {
		return result, err
	}
	if err := approvalledger.Request(ctx, tx, request.id, row.IntentID, domain.FormatTime(now), domain.FormatTime(expiresAt), request.json, request.nonce); err != nil {
		return result, fmt.Errorf("insert approval: %w", err)
	}
	return g.publishApprovalRequest(ctx, tx, row, request, result, now)
}

func (g *Gateway) prepareApprovalRequest(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, expiresAt time.Time) (approvalRequestDocument, error) {
	request := approvalRequestDocument{id: g.idGen.New(ids.PrefixApproval)}
	nonceDigest := sha256.Sum256([]byte(request.id + "|" + row.IntentID))
	request.nonce = hex.EncodeToString(nonceDigest[:])
	var err error
	request.data, err = approvalNotificationData(ctx, tx, row, request.id, expiresAt, intent)
	if err != nil {
		return request, fmt.Errorf("build approval notification: %w", err)
	}
	request.json, err = approvalRequestJSON(row, intent, request)
	return request, err
}

func approvalRequestJSON(row intentRow, intent map[string]any, request approvalRequestDocument) ([]byte, error) {
	raw, err := canonicaljson.Marshal(map[string]any{
		"approval_id": request.id, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"tenant_id": row.TenantID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "risk_class": row.RiskClass,
		"intent": intent, "notification": request.data, "nonce": request.nonce,
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize approval: %w", err)
	}
	return raw, nil
}

func (g *Gateway) publishApprovalRequest(ctx context.Context, tx *sql.Tx, row intentRow, request approvalRequestDocument, result Result, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approval_required', updated_at = ? WHERE intent_id = ?", domain.FormatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("mark approval required: %w", err)
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.requested:"+request.id, row.TenantID, notify.TypeApprovalRequested, "approval/"+request.id, row.SituationID, request.data, now, traceContext(row)); err != nil {
		return result, fmt.Errorf("append approval requested notification: %w", err)
	}
	result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", request.id
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func pendingApproval(ctx context.Context, tx *sql.Tx, intentID string) (string, error) {
	var approvalID string
	err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", intentID).Scan(&approvalID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find pending approval: %w", err)
	}
	return approvalID, nil
}
