package approvalledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

// WithdrawSuperseded withdraws pending approvals bound to an older Situation version.
func WithdrawSuperseded(ctx context.Context, tx *sql.Tx, situationID, tenantID string, replacementVersion int, now string, clk clock.Clock) error {
	rows, err := findSupersededApprovals(ctx, tx, situationID, replacementVersion)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	return withdrawSupersededRows(ctx, tx, rows, tenantID, now, clk)
}

func findSupersededApprovals(ctx context.Context, tx *sql.Tx, situationID string, replacementVersion int) (*sql.Rows, error) {
	rows, err := tx.QueryContext(ctx, `
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
	return rows, nil
}

func withdrawSupersededRows(ctx context.Context, tx *sql.Tx, rows *sql.Rows, tenantID, now string, clk clock.Clock) error {
	for rows.Next() {
		if err := withdrawSupersededRow(ctx, tx, rows, tenantID, now, clk); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate superseded approvals: %w", err)
	}
	return nil
}

// withdrawSupersededRow withdraws the approval at the cursor and publishes
// its withdrawal notification in the same transaction.
func withdrawSupersededRow(ctx context.Context, tx *sql.Tx, rows *sql.Rows, tenantID, now string, clk clock.Clock) error {
	approval, err := scanSupersededApproval(rows)
	if err != nil {
		return err
	}
	if err := Withdraw(ctx, tx, approval.approvalID, now); err != nil {
		return fmt.Errorf("withdraw superseded approval %s: %w", approval.approvalID, err)
	}
	return publishSupersededWithdrawal(ctx, tx, approval, tenantID, clk)
}

type supersededApproval struct {
	approvalID, intentID, situationID string
	situationVersion                  int
	traceparent, tracestate           sql.NullString
}

func scanSupersededApproval(rows *sql.Rows) (supersededApproval, error) {
	var approval supersededApproval
	if err := rows.Scan(&approval.approvalID, &approval.intentID, &approval.situationID,
		&approval.situationVersion, &approval.traceparent, &approval.tracestate); err != nil {
		return supersededApproval{}, fmt.Errorf("scan superseded approval: %w", err)
	}
	return approval, nil
}

func publishSupersededWithdrawal(ctx context.Context, tx *sql.Tx, approval supersededApproval, tenantID string, clk clock.Clock) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.withdrawn:"+approval.approvalID,
		tenantID, notify.TypeApprovalWithdrawn, "approval/"+approval.approvalID, approval.situationID, map[string]any{
			"tenant_id": tenantID, "approval_id": approval.approvalID, "intent_id": approval.intentID, "situation_id": approval.situationID,
			"situation_version": approval.situationVersion, "reason": "situation_version_conflict", "source_authority": notify.SourceForTenant(tenantID),
		}, clk.Now().UTC(), contractsv1.TraceContext{Traceparent: approval.traceparent.String, Tracestate: approval.tracestate.String}); err != nil {
		return fmt.Errorf("append superseded approval notification: %w", err)
	}
	return nil
}
