package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
)

type Reader struct{ db *sql.DB }

func NewReader(db *sql.DB) Reader { return Reader{db: db} }

const approvalsSQL = `
	SELECT approval_id, status, requested_at, expires_at, COALESCE(decided_at, ''), COALESCE(approver_identity, ''),
		COALESCE(relay_identity, ''), COALESCE(reason, ''), COALESCE(withdrawn_at, ''), COALESCE(withdrawal_reason, '')
	FROM approvals WHERE intent_id = ? ORDER BY requested_at, approval_id`

func (r Reader) Approvals(ctx context.Context, intentID string) ([]domain.ApprovalView, error) {
	rows, err := r.db.QueryContext(ctx, approvalsSQL, intentID)
	if err != nil {
		return nil, fmt.Errorf("read approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	approvals, err := storage.CollectRows(rows, "approvals", scanApprovalView)
	if err != nil {
		return nil, fmt.Errorf("read approvals: %w", err)
	}
	return approvals, nil
}

func scanApprovalView(rows *sql.Rows) (domain.ApprovalView, error) {
	var a domain.ApprovalView
	if err := rows.Scan(&a.ApprovalID, &a.Status, &a.RequestedAt, &a.ExpiresAt, &a.DecidedAt, &a.Approver, &a.Relay, &a.Reason, &a.WithdrawnAt, &a.WithdrawalReason); err != nil {
		return domain.ApprovalView{}, fmt.Errorf("scan approval: %w", err)
	}
	return a, nil
}
