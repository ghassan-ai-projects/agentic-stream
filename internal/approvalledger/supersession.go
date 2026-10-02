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
	rows, err := tx.QueryContext(ctx, `
		SELECT a.approval_id, i.intent_id, i.situation_id, i.situation_version,
		       d.traceparent, d.tracestate
		FROM approvals a
		JOIN intents i ON i.intent_id = a.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE i.situation_id = ? AND i.situation_version < ? AND a.status = 'pending'
		ORDER BY a.approval_id`, situationID, replacementVersion)
	if err != nil {
		return fmt.Errorf("find superseded approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var approvalID, intentID, linkedSituationID string
		var situationVersion int
		var traceparent, tracestate sql.NullString
		if err := rows.Scan(&approvalID, &intentID, &linkedSituationID, &situationVersion, &traceparent, &tracestate); err != nil {
			return fmt.Errorf("scan superseded approval: %w", err)
		}
		if err := Withdraw(ctx, tx, approvalID, now); err != nil {
			return fmt.Errorf("withdraw superseded approval %s: %w", approvalID, err)
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.withdrawn:"+approvalID, tenantID, notify.TypeApprovalWithdrawn, "approval/"+approvalID, linkedSituationID, map[string]any{
			"tenant_id": tenantID, "approval_id": approvalID, "intent_id": intentID, "situation_id": linkedSituationID,
			"situation_version": situationVersion, "reason": "situation_version_conflict", "source_authority": notify.SourceForTenant(tenantID),
		}, clk.Now().UTC(), contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}); err != nil {
			return fmt.Errorf("append superseded approval notification: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate superseded approvals: %w", err)
	}
	return nil
}
