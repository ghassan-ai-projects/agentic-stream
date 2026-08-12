package cognition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

const reconsiderationTrigger = "prior_action_invalidated"

// admitReconsiderations detects corrections that supersede a version whose
// accepted Intent produced a succeeded Command. The unique database key makes
// replay and duplicate correction delivery idempotent.
func (e *Engine) admitReconsiderations(ctx context.Context, tx *sql.Tx, current situations.Version) (int, error) {
	if current.Completeness != "corrected" || current.PreviousVersion < 1 || e.spec.Time.LatePolicy != "correct_and_reconsider" {
		return 0, nil
	}
	if e.spec.Digest == "" {
		return 0, fmt.Errorf("compiled spec has no digest")
	}
	var correction map[string]any
	if err := json.Unmarshal(current.SnapshotJSON, &correction); err != nil {
		return 0, fmt.Errorf("decode correction snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, correction); err != nil {
		return 0, fmt.Errorf("validate correction snapshot: %w", err)
	}
	correctionDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, correction)
	if err != nil {
		return 0, fmt.Errorf("digest correction snapshot: %w", err)
	}
	var persistedCorrectionDigest []byte
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", current.SituationID, current.Version).Scan(&persistedCorrectionDigest); err != nil {
		return 0, fmt.Errorf("load correction snapshot digest: %w", err)
	}
	decodedCorrectionDigest, err := canonicaljson.DecodeDigest(correctionDigest)
	if err != nil || !bytes.Equal(decodedCorrectionDigest, persistedCorrectionDigest) {
		return 0, fmt.Errorf("correction snapshot digest mismatch")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT c.command_id, d.decision_id, o.outcome_id, o.ordinal, o.status, o.provider_result_json,
		       o.observed_effect_json, o.reconciliation_status, o.outcome_sha256
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN outcomes o ON o.command_id = c.command_id
		WHERE i.situation_id = ? AND i.situation_version = ?
		  AND d.validation_status = 'accepted' AND c.status = 'succeeded'
		  AND o.ordinal = (SELECT MAX(o2.ordinal) FROM outcomes o2 WHERE o2.command_id = c.command_id)
		ORDER BY c.command_id`, current.SituationID, current.PreviousVersion)
	if err != nil {
		return 0, fmt.Errorf("find invalidated commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	admitted := 0
	for rows.Next() {
		var commandID, decisionID, outcomeID, outcomeStatus, reconciliationStatus string
		var outcomeOrdinal int
		var providerJSON, observedJSON, outcomeSHA []byte
		if err := rows.Scan(&commandID, &decisionID, &outcomeID, &outcomeOrdinal, &outcomeStatus, &providerJSON, &observedJSON, &reconciliationStatus, &outcomeSHA); err != nil {
			return admitted, fmt.Errorf("scan invalidated command: %w", err)
		}
		var existing int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM reconsiderations WHERE situation_id = ? AND superseded_version = ? AND invalidated_command_id = ?`, current.SituationID, current.PreviousVersion, commandID).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return admitted, fmt.Errorf("check reconsideration dedupe: %w", err)
		}
		material := fmt.Sprintf("reconsider|%s|%d|%s", current.SituationID, current.PreviousVersion, commandID)
		key := sha256.Sum256([]byte(material))
		reconsiderationID := ids.PrefixReconsideration + hex.EncodeToString(key[:])
		triggerID := "trg_reconsider_" + hex.EncodeToString(key[:])
		schedulerItemID := "sch_reconsider_" + hex.EncodeToString(key[:])
		priorOutcome := map[string]any{"status": outcomeStatus, "reconciliation_status": reconciliationStatus}
		priorOutcome["outcome_id"] = outcomeID
		priorOutcome["ordinal"] = outcomeOrdinal
		priorOutcome["outcome_sha256"] = "sha256:" + hex.EncodeToString(outcomeSHA)
		if len(providerJSON) > 0 {
			priorOutcome["provider_result"] = json.RawMessage(providerJSON)
		}
		if len(observedJSON) > 0 {
			priorOutcome["observed_effect"] = json.RawMessage(observedJSON)
		}
		delta := map[string]any{
			"reason":                 reconsiderationTrigger,
			"correction":             correction,
			"superseded_version":     current.PreviousVersion,
			"correction_version":     current.Version,
			"invalidated_command_id": commandID,
			"prior_decision_id":      decisionID,
			"prior_outcome":          priorOutcome,
		}
		deltaJSON, err := canonicaljson.Marshal(delta)
		if err != nil {
			return admitted, fmt.Errorf("canonicalize reconsideration evidence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reconsiderations (
				reconsideration_id, tenant_id, situation_id, superseded_version,
				correction_version, correction_snapshot_sha256, invalidated_command_id,
				invalidated_outcome_id, invalidated_outcome_sha256, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			reconsiderationID, e.tenantID, current.SituationID, current.PreviousVersion,
			current.Version, decodedCorrectionDigest, commandID, outcomeID, outcomeSHA,
			e.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return admitted, fmt.Errorf("insert reconsideration: %w", err)
		}
		eval := Evaluation{
			TriggerID: triggerID, TriggerName: reconsiderationTrigger,
			SituationID: current.SituationID, SituationVersion: current.Version,
			Score: 100, Threshold: 0, Lane: "deep", Outcome: "admitted",
			Reasons:      []string{"accepted action invalidated by corrected Situation version"},
			PolicySHA256: e.spec.Digest, DeltaJSON: deltaJSON, EvaluatedAt: e.clk.Now().UTC(),
		}
		if err := e.scheduler.saveEvaluation(ctx, tx, eval, e.tenantID, e.deploymentID); err != nil {
			return admitted, fmt.Errorf("save reconsideration evaluation: %w", err)
		}
		item := Item{
			SchedulerItemID: schedulerItemID, Kind: "reconsider", TriggerID: triggerID,
			SituationID: current.SituationID, SituationVersion: current.Version,
			Lane: "deep", Priority: 100, Status: "pending",
			ExpiresAt: e.clk.Now().UTC().Add(defaultExpiresAfter),
		}
		if err := e.scheduler.insertItem(ctx, tx, item, e.tenantID); err != nil {
			return admitted, fmt.Errorf("insert reconsideration item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reconsiderations SET trigger_id = ?, scheduler_item_id = ? WHERE reconsideration_id = ?`, triggerID, schedulerItemID, reconsiderationID); err != nil {
			return admitted, fmt.Errorf("link reconsideration admission: %w", err)
		}
		admitted++
	}
	if err := rows.Err(); err != nil {
		return admitted, fmt.Errorf("iterate invalidated commands: %w", err)
	}
	return admitted, nil
}
