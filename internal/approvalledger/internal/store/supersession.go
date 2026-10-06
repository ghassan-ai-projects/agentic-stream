package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
)

// SupersededApprovals lists the pending approvals bound to a Situation version
// older than the replacement, in approval id order. It reads the intents and
// decisions that bind an approval to a Situation version and closes the result
// set before the caller writes anything.
func (t *Tx) SupersededApprovals(ctx context.Context, situationID string, replacementVersion int) ([]domain.Withdrawal, error) {
	rows, err := t.tx.QueryContext(ctx, `
		SELECT a.approval_id, i.intent_id, i.situation_id, i.situation_version,
		       d.traceparent, d.tracestate
		FROM approvals a
		JOIN intents i ON i.intent_id = a.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE i.situation_id = ? AND i.situation_version < ? AND a.status = 'pending'
		ORDER BY a.approval_id`, situationID, replacementVersion)
	if err != nil {
		return nil, fmt.Errorf("find superseded approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectWithdrawals(rows)
}

func collectWithdrawals(rows *sql.Rows) ([]domain.Withdrawal, error) {
	var withdrawals []domain.Withdrawal
	for rows.Next() {
		var w domain.Withdrawal
		var traceparent, tracestate sql.NullString
		if err := rows.Scan(&w.ApprovalID, &w.IntentID, &w.SituationID, &w.SituationVersion, &traceparent, &tracestate); err != nil {
			return nil, fmt.Errorf("scan superseded approval: %w", err)
		}
		w.Traceparent, w.Tracestate = traceparent.String, tracestate.String
		withdrawals = append(withdrawals, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate superseded approvals: %w", err)
	}
	return withdrawals, nil
}
