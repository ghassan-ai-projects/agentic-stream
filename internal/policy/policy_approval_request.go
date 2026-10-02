package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func (g *Gateway) requireApproval(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	if approvalID, err := pendingApproval(ctx, tx, row.IntentID); err != nil {
		return result, err
	} else if approvalID != "" {
		result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}
	approvalID := g.idGen.New(ids.PrefixApproval)
	nonceDigest := sha256.Sum256([]byte(approvalID + "|" + row.IntentID))
	nonce := hex.EncodeToString(nonceDigest[:])
	approvalData, err := approvalNotificationData(ctx, tx, row, approvalID, expiresAt, intent)
	if err != nil {
		return result, fmt.Errorf("build approval notification: %w", err)
	}
	approvalJSON, err := canonicaljson.Marshal(map[string]any{
		"approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"tenant_id": row.TenantID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "risk_class": row.RiskClass,
		"intent": intent, "notification": approvalData, "nonce": nonce,
	})
	if err != nil {
		return result, fmt.Errorf("canonicalize approval: %w", err)
	}
	if err := approvalledger.Request(ctx, tx, approvalID, row.IntentID, formatTime(now), formatTime(expiresAt), approvalJSON, nonce); err != nil {
		return result, fmt.Errorf("insert approval: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approval_required', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("mark approval required: %w", err)
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.requested:"+approvalID, row.TenantID, notify.TypeApprovalRequested, "approval/"+approvalID, row.SituationID, approvalData, now, traceContext(row)); err != nil {
		return result, fmt.Errorf("append approval requested notification: %w", err)
	}
	result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
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

func approvalNotificationData(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, expiresAt time.Time, intent map[string]any) (map[string]any, error) {
	if len(row.IntentSHA) != sha256.Size {
		return nil, fmt.Errorf("intent digest is incomplete")
	}
	snapshotSHA, err := approvalSnapshotDigest(ctx, tx, row)
	if err != nil {
		return nil, err
	}
	delta, err := approvalDelta(ctx, tx, row.EpisodeID)
	if err != nil {
		return nil, err
	}
	decision, err := decodeApprovalDecision(row.DecisionJSON)
	if err != nil {
		return nil, err
	}
	evidence := intentEvidence(intent)
	if parameters, ok := intent["parameters"].(map[string]any); !ok || parameters == nil {
		intent["parameters"] = map[string]any{}
	}
	summary := documentString(decision, "summary")
	if strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("Decision %s requires approval", row.DecisionID)
	}
	hypothesis := documentString(decision, "primary_hypothesis")
	if strings.TrimSpace(hypothesis) == "" {
		hypothesis = fmt.Sprintf("Decision %s did not record a primary hypothesis", row.DecisionID)
	}
	return map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"situation_id": row.SituationID, "situation_version": row.SituationVersion,
		"intent_digest":   "sha256:" + hex.EncodeToString(row.IntentSHA),
		"snapshot_digest": "sha256:" + hex.EncodeToString(snapshotSHA), "risk_class": row.RiskClass,
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano), "audience": "stream-approval-relay",
		"summary": summary, "delta": delta, "hypothesis": hypothesis, "evidence": evidence,
		"action": intent["parameters"], "decline_consequence": "The intent will not be dispatched.",
		"source_authority": notify.SourceForTenant(row.TenantID),
	}, nil
}

func approvalSnapshotDigest(ctx context.Context, tx *sql.Tx, row intentRow) ([]byte, error) {
	var digest []byte
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", row.SituationID, row.SituationVersion).Scan(&digest); err != nil {
		return nil, fmt.Errorf("load approval snapshot digest: %w", err)
	}
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("approval snapshot digest is incomplete")
	}
	return digest, nil
}

func approvalDelta(ctx context.Context, tx *sql.Tx, episodeID string) (map[string]any, error) {
	delta := map[string]any{}
	var raw []byte
	err := tx.QueryRowContext(ctx, `
		SELECT te.delta_json
		FROM trigger_evaluations te
		JOIN scheduler_items si ON si.trigger_id = te.trigger_id
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		WHERE e.episode_id = ?
		ORDER BY te.evaluated_at DESC LIMIT 1`, episodeID).Scan(&raw)
	if err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &delta); err != nil {
			return nil, fmt.Errorf("decode approval delta: %w", err)
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load approval delta: %w", err)
	}
	if delta == nil {
		return nil, fmt.Errorf("approval delta must be an object")
	}
	return delta, nil
}

func decodeApprovalDecision(raw []byte) (map[string]any, error) {
	var decision map[string]any
	if err := json.Unmarshal(raw, &decision); err != nil {
		return nil, fmt.Errorf("decode approval decision: %w", err)
	}
	return decision, nil
}

func intentEvidence(intent map[string]any) []string {
	values, _ := intent["evidence_ids"].([]any)
	evidence := make([]string, 0, len(values))
	for _, value := range values {
		if id, ok := value.(string); ok && id != "" {
			evidence = append(evidence, id)
		}
	}
	return evidence
}
