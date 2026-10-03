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

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

type approvalContext struct {
	snapshot        []byte
	delta, decision map[string]any
}

const loadApprovalDeltaSQL = `
		SELECT te.delta_json
		FROM trigger_evaluations te
		JOIN scheduler_items si ON si.trigger_id = te.trigger_id
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		WHERE e.episode_id = ?
		ORDER BY te.evaluated_at DESC LIMIT 1`

func approvalNotificationData(ctx context.Context, tx *sql.Tx, row intentRow, approvalID string, expiresAt time.Time, intent map[string]any) (map[string]any, error) {
	if len(row.IntentSHA) != sha256.Size {
		return nil, fmt.Errorf("intent digest is incomplete")
	}
	context, err := loadApprovalContext(ctx, tx, row)
	if err != nil {
		return nil, err
	}
	return buildApprovalNotification(row, approvalID, expiresAt, intent, context), nil
}

func loadApprovalContext(ctx context.Context, tx *sql.Tx, row intentRow) (approvalContext, error) {
	snapshotSHA, err := approvalSnapshotDigest(ctx, tx, row)
	if err != nil {
		return approvalContext{}, err
	}
	delta, err := approvalDelta(ctx, tx, row.EpisodeID)
	if err != nil {
		return approvalContext{}, err
	}
	decision, err := decodeApprovalDecision(row.DecisionJSON)
	if err != nil {
		return approvalContext{}, err
	}
	return approvalContext{snapshotSHA, delta, decision}, nil
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
	err := tx.QueryRowContext(ctx, loadApprovalDeltaSQL, episodeID).Scan(&raw)
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

func buildApprovalNotification(row intentRow, approvalID string, expiresAt time.Time, intent map[string]any, context approvalContext) map[string]any {
	evidence := intentEvidence(intent)
	if parameters, ok := intent["parameters"].(map[string]any); !ok || parameters == nil {
		intent["parameters"] = map[string]any{}
	}
	summary := approvalDecisionText(context.decision, "summary", "Decision %s requires approval", row.DecisionID)
	hypothesis := approvalDecisionText(context.decision, "primary_hypothesis", "Decision %s did not record a primary hypothesis", row.DecisionID)
	return approvalNotificationFields(row, approvalID, expiresAt, intent, context, evidence, summary, hypothesis)
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

func approvalDecisionText(decision map[string]any, field, fallback, decisionID string) string {
	value := documentString(decision, field)
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf(fallback, decisionID)
	}
	return value
}

func approvalNotificationFields(row intentRow, approvalID string, expiresAt time.Time, intent map[string]any, context approvalContext, evidence []string, summary, hypothesis string) map[string]any {
	return map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"situation_id": row.SituationID, "situation_version": row.SituationVersion,
		"intent_digest":   "sha256:" + hex.EncodeToString(row.IntentSHA),
		"snapshot_digest": "sha256:" + hex.EncodeToString(context.snapshot), "risk_class": row.RiskClass,
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano), "audience": "stream-approval-relay",
		"summary": summary, "delta": context.delta, "hypothesis": hypothesis, "evidence": evidence,
		"action": intent["parameters"], "decline_consequence": "The intent will not be dispatched.",
		"source_authority": notify.SourceForTenant(row.TenantID),
	}
}
