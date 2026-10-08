package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
)

// Reader reads approvals without a transaction.
type Reader struct{ db *sql.DB }

// NewReader binds an approval reader to an open database.
func NewReader(db *sql.DB) Reader { return Reader{db: db} }

// Approvals reads every approval request of one intent, oldest first.
func (r Reader) Approvals(ctx context.Context, intentID string) ([]domain.ApprovalView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT approval_id, status, requested_at, expires_at, COALESCE(decided_at, ''), COALESCE(approver_identity, ''),
			COALESCE(relay_identity, ''), COALESCE(reason, ''), COALESCE(withdrawn_at, ''), COALESCE(withdrawal_reason, '')
		FROM approvals WHERE intent_id = ? ORDER BY requested_at, approval_id`, intentID)
	if err != nil {
		return nil, fmt.Errorf("read approvals: %w", err)
	}
	return scanApprovalViews(rows)
}

func scanApprovalViews(rows *sql.Rows) ([]domain.ApprovalView, error) {
	defer func() { _ = rows.Close() }()
	approvals := []domain.ApprovalView{}
	for rows.Next() {
		var a domain.ApprovalView
		if err := rows.Scan(&a.ApprovalID, &a.Status, &a.RequestedAt, &a.ExpiresAt, &a.DecidedAt, &a.Approver, &a.Relay, &a.Reason, &a.WithdrawnAt, &a.WithdrawalReason); err != nil {
			return nil, fmt.Errorf("scan approval: %w", err)
		}
		approvals = append(approvals, a)
	}
	return approvals, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}
