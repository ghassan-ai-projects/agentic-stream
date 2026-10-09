package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SetIntentStatus updates only policy's governance columns.
func (tx *Tx) SetIntentStatus(ctx context.Context, c domain.IntentStatusChange) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", c.Status, kernel.FormatTime(c.Now), c.IntentID); err != nil {
		return fmt.Errorf("%s: %w", c.Operation, err)
	}
	return nil
}

// RecordEvaluation persists the evidence for a completed evaluation.
func (tx *Tx) RecordEvaluation(ctx context.Context, a domain.EvaluationAudit) error {
	r := a.Row
	if _, err := tx.tx.ExecContext(ctx, recordEvaluationSQL, a.ID, r.IntentID, r.DecisionID, a.PolicyVersion, a.Result.Result, a.PolicyDigest, r.IntentSHA, r.DecisionSHA, storage.NullIfEmpty(a.Result.CommandID), storage.NullIfEmpty(a.Result.ApprovalID), a.Reason, r.CurrentSituation, kernel.FormatTime(a.Now)); err != nil {
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
