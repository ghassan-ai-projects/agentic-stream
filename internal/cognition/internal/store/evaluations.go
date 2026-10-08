package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (t *Tx) LoadEvaluationReasons(ctx context.Context, schedulerItemID string) (string, []string, error) {
	var triggerID string
	var reasonsJSON []byte
	if err := t.tx.QueryRowContext(ctx, `
			SELECT trigger_id, reasons_json FROM trigger_evaluations
			WHERE trigger_id = (SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?)`, schedulerItemID).
		Scan(&triggerID, &reasonsJSON); err != nil {
		return "", nil, fmt.Errorf("load cost-rejected trigger evaluation: %w", err)
	}
	var reasons []string
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &reasons); err != nil {
			return "", nil, fmt.Errorf("decode trigger evaluation reasons: %w", err)
		}
	}
	return triggerID, reasons, nil
}

func (t *Tx) UpsertEvaluation(ctx context.Context, eval domain.Evaluation, tenantID, deploymentID, policyDigest string) error {
	columns, err := evaluationColumns(eval, tenantID, deploymentID, policyDigest)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, upsertEvaluationSQL, columns...); err != nil {
		return fmt.Errorf("upsert trigger evaluation: %w", err)
	}
	return nil
}

func evaluationColumns(eval domain.Evaluation, tenantID, deploymentID, policyDigest string) ([]any, error) {
	reasonsJSON, err := json.Marshal(eval.Reasons)
	if err != nil {
		return nil, fmt.Errorf("marshal reasons: %w", err)
	}
	policySHA, err := canonicaljson.DecodeDigest(policyDigest)
	if err != nil {
		return nil, fmt.Errorf("decode policy digest: %w", err)
	}
	return []any{
		eval.TriggerID, tenantID, deploymentID, eval.TriggerName,
		eval.SituationID, eval.SituationVersion, eval.Score, eval.Threshold, eval.Lane,
		eval.Outcome, reasonsJSON, policySHA[:], eval.DeltaJSON, eval.EvaluatedAt.Format(time.RFC3339Nano),
	}, nil
}

const upsertEvaluationSQL = `
		INSERT INTO trigger_evaluations (
			trigger_id, tenant_id, deployment_id, trigger_name,
			situation_id, situation_version, score, threshold, lane,
			outcome, reasons_json, policy_sha256, delta_json, evaluated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id) DO UPDATE SET
			situation_version = excluded.situation_version,
			score = excluded.score,
			threshold = excluded.threshold,
			lane = excluded.lane,
			outcome = excluded.outcome,
			reasons_json = excluded.reasons_json,
			policy_sha256 = excluded.policy_sha256,
			delta_json = excluded.delta_json,
			evaluated_at = excluded.evaluated_at`

func (t *Tx) AnnounceEvaluation(ctx context.Context, eval domain.Evaluation, tenantID string) error {
	event := evaluationEvent(eval, tenantID)
	var err error
	if event.EnvelopeDigest, err = event.ComputeEnvelopeDigest(); err != nil {
		return fmt.Errorf("digest trigger notification: %w", err)
	}
	if _, err := notify.Append(ctx, t.tx, event, eval.EvaluatedAt); err != nil {
		return fmt.Errorf("append trigger notification: %w", err)
	}
	return nil
}

func evaluationEvent(eval domain.Evaluation, tenantID string) contractsv1.CloudEvent {
	return contractsv1.CloudEvent{
		SpecVersion: "1.0", ID: eval.TriggerID + ":" + eval.Outcome + ":" + eval.EvaluatedAt.UTC().Format(time.RFC3339Nano), Source: "//agentic-stream/tenants/" + tenantID,
		Type: "situation.trigger.evaluated", Subject: "situation/" + eval.SituationID,
		Time: eval.EvaluatedAt, DataContentType: "application/json",
		DataSchema: "urn:situation-runtime:schema:trigger-evaluation:v1",
		Data:       map[string]any{"trigger_id": eval.TriggerID, "situation_id": eval.SituationID, "situation_version": eval.SituationVersion, "outcome": eval.Outcome},
		TenantID:   tenantID, PartitionKey: eval.SituationID, IngestedTime: eval.EvaluatedAt,
		Classification: contractsv1.ClassificationInternal,
	}
}

const (
	markReasonedSQL = "UPDATE situations SET last_reasoned_version = ? WHERE situation_id = ?"
	markMaterialSQL = "UPDATE situations SET last_material_version = ? WHERE situation_id = ?"
)

func (t *Tx) MarkVersionReasoned(ctx context.Context, v situations.Version) error {
	return t.markVersion(ctx, markReasonedSQL, "reasoned", v)
}

func (t *Tx) MarkVersionMaterial(ctx context.Context, v situations.Version) error {
	return t.markVersion(ctx, markMaterialSQL, "material", v)
}

func (t *Tx) markVersion(ctx context.Context, statement, kind string, v situations.Version) error {
	if _, err := t.tx.ExecContext(ctx, statement, v.Version, v.SituationID); err != nil {
		return fmt.Errorf("update last %s version: %w", kind, err)
	}
	return nil
}

func (t *Tx) RecordCostReason(ctx context.Context, triggerID string, reasons []string) error {
	encoded, err := json.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("encode trigger evaluation reasons: %w", err)
	}
	if _, err := t.tx.ExecContext(ctx, "UPDATE trigger_evaluations SET reasons_json = ? WHERE trigger_id = ?", encoded, triggerID); err != nil {
		return fmt.Errorf("record cost rejection reason: %w", err)
	}
	return nil
}
