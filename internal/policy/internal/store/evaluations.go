package store

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"time"
)

// SetIntentStatus updates only policy's governance columns.
func (tx *Tx) SetIntentStatus(ctx context.Context, intentID, status string, now time.Time, operation string) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", status, domain.FormatTime(now), intentID); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// RecordEvaluation persists the evidence for a completed evaluation.
func (tx *Tx) RecordEvaluation(ctx context.Context, a domain.EvaluationAudit) error {
	r := a.Row
	if _, err := tx.tx.ExecContext(ctx, recordEvaluationSQL, a.ID, r.IntentID, r.DecisionID, a.PolicyVersion, a.Result.Result, a.PolicyDigest, r.IntentSHA, r.DecisionSHA, nullableID(a.Result.CommandID), nullableID(a.Result.ApprovalID), a.Reason, r.CurrentSituation, domain.FormatTime(a.Now)); err != nil {
		return fmt.Errorf("record policy evaluation: %w", err)
	}
	return nil
}

const recordEvaluationSQL = `INSERT INTO policy_evaluations (
 evaluation_id,intent_id,decision_id,policy_version,result,policy_digest,intent_sha256,decision_sha256,command_id,approval_id,reason,situation_version,evaluated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`

// Failure context names preserve the public operation's existing errors.
const (
	SetPolicyStatus       = "set intent policy status"
	ApproveIntentStatus   = "approve intent"
	RequireApprovalStatus = "mark approval required"
	MarkStaleStatus       = "mark stale intent"
	ResolveIntentStatus   = "update approved intent"
)
