package cognition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
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
	correction, correctionDigest, err := verifiedCorrection(ctx, tx, current)
	if err != nil {
		return 0, err
	}
	invalidated, err := invalidatedCommands(ctx, tx, current)
	if err != nil {
		return 0, err
	}
	admitted := 0
	for _, command := range invalidated {
		created, err := e.admitReconsideration(ctx, tx, current, correction, correctionDigest, command)
		if err != nil {
			return admitted, err
		}
		if created {
			admitted++
		}
	}
	return admitted, nil
}

// verifiedCorrection decodes the corrected snapshot and requires it to match
// the digest persisted with its Situation version.
func verifiedCorrection(ctx context.Context, tx *sql.Tx, current situations.Version) (map[string]any, []byte, error) {
	var correction map[string]any
	if err := json.Unmarshal(current.SnapshotJSON, &correction); err != nil {
		return nil, nil, fmt.Errorf("decode correction snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, correction); err != nil {
		return nil, nil, fmt.Errorf("validate correction snapshot: %w", err)
	}
	correctionDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, correction)
	if err != nil {
		return nil, nil, fmt.Errorf("digest correction snapshot: %w", err)
	}
	var persistedCorrectionDigest []byte
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", current.SituationID, current.Version).Scan(&persistedCorrectionDigest); err != nil {
		return nil, nil, fmt.Errorf("load correction snapshot digest: %w", err)
	}
	decodedCorrectionDigest, err := canonicaljson.DecodeDigest(correctionDigest)
	if err != nil || !bytes.Equal(decodedCorrectionDigest, persistedCorrectionDigest) {
		return nil, nil, fmt.Errorf("correction snapshot digest mismatch")
	}
	return correction, decodedCorrectionDigest, nil
}

// invalidatedCommand is a succeeded command, with its latest outcome, whose
// authorizing Situation version the correction superseded.
type invalidatedCommand struct {
	commandID, decisionID, outcomeID, outcomeStatus, reconciliationStatus string
	outcomeOrdinal                                                        int
	providerJSON, observedJSON, outcomeSHA                                []byte
}

func invalidatedCommands(ctx context.Context, tx *sql.Tx, current situations.Version) ([]invalidatedCommand, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.command_id, d.decision_id, o.outcome_id, o.ordinal, o.status, o.provider_result_json,
		       o.observed_effect_json, o.reconciliation_status, o.outcome_sha256
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN outcomes o ON o.command_id = c.command_id
		WHERE i.situation_id = ?
		  AND i.situation_version = (
			SELECT MAX(i2.situation_version)
			FROM intents i2
			WHERE i2.situation_id = i.situation_id
			  AND i2.situation_version <= ?
			  AND i2.policy_status = 'approved'
		  )
		  AND d.validation_status = 'accepted' AND c.status = 'succeeded'
		  AND o.ordinal = (SELECT MAX(o2.ordinal) FROM outcomes o2 WHERE o2.command_id = c.command_id)
		ORDER BY c.command_id`, current.SituationID, current.PreviousVersion)
	if err != nil {
		return nil, fmt.Errorf("find invalidated commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var commands []invalidatedCommand
	for rows.Next() {
		var c invalidatedCommand
		if err := rows.Scan(&c.commandID, &c.decisionID, &c.outcomeID, &c.outcomeOrdinal, &c.outcomeStatus, &c.providerJSON, &c.observedJSON, &c.reconciliationStatus, &c.outcomeSHA); err != nil {
			return nil, fmt.Errorf("scan invalidated command: %w", err)
		}
		commands = append(commands, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invalidated commands: %w", err)
	}
	return commands, nil
}

// priorOutcome is the invalidated command's latest outcome as reconsideration
// evidence.
func (c invalidatedCommand) priorOutcome() map[string]any {
	outcome := map[string]any{
		"status": c.outcomeStatus, "reconciliation_status": c.reconciliationStatus,
		"outcome_id": c.outcomeID, "ordinal": c.outcomeOrdinal,
		"outcome_sha256": "sha256:" + hex.EncodeToString(c.outcomeSHA),
	}
	if len(c.providerJSON) > 0 {
		outcome["provider_result"] = json.RawMessage(c.providerJSON)
	}
	if len(c.observedJSON) > 0 {
		outcome["observed_effect"] = json.RawMessage(c.observedJSON)
	}
	return outcome
}

// admitReconsideration records one reconsideration and admits its deep-lane
// episode. Identities derive from the Situation, superseded version, and
// command, so a duplicate correction delivery admits nothing.
func (e *Engine) admitReconsideration(ctx context.Context, tx *sql.Tx, current situations.Version, correction map[string]any, correctionDigest []byte, command invalidatedCommand) (bool, error) {
	exists, err := reconsiderationExists(ctx, tx, current, command.commandID)
	if err != nil || exists {
		return false, err
	}
	r := newReconsideration(current, command)
	deltaJSON, err := r.evidenceJSON(correction)
	if err != nil {
		return false, err
	}
	if err := e.recordReconsideration(ctx, tx, r, correctionDigest); err != nil {
		return false, err
	}
	if err := e.scheduleReconsideration(ctx, tx, r, deltaJSON); err != nil {
		return false, err
	}
	if err := e.announceReconsideration(ctx, tx, r); err != nil {
		return false, err
	}
	return true, nil
}

func reconsiderationExists(ctx context.Context, tx *sql.Tx, current situations.Version, commandID string) (bool, error) {
	var existing int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM reconsiderations WHERE situation_id = ? AND superseded_version = ? AND invalidated_command_id = ?`, current.SituationID, current.PreviousVersion, commandID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check reconsideration dedupe: %w", err)
	}
	return true, nil
}

// reconsideration is one invalidated command's reconsideration with its
// deterministic identities.
type reconsideration struct {
	current                                       situations.Version
	command                                       invalidatedCommand
	reconsiderationID, triggerID, schedulerItemID string
}

func newReconsideration(current situations.Version, command invalidatedCommand) reconsideration {
	material := fmt.Sprintf("reconsider|%s|%d|%s", current.SituationID, current.PreviousVersion, command.commandID)
	sum := sha256.Sum256([]byte(material))
	key := hex.EncodeToString(sum[:])
	return reconsideration{
		current: current, command: command,
		reconsiderationID: ids.PrefixReconsideration + key,
		triggerID:         "trg_reconsider_" + key,
		schedulerItemID:   "sch_reconsider_" + key,
	}
}

func (r reconsideration) evidenceJSON(correction map[string]any) ([]byte, error) {
	deltaJSON, err := canonicaljson.Marshal(map[string]any{
		"reason":                 reconsiderationTrigger,
		"correction":             correction,
		"superseded_version":     r.current.PreviousVersion,
		"correction_version":     r.current.Version,
		"invalidated_command_id": r.command.commandID,
		"prior_decision_id":      r.command.decisionID,
		"prior_outcome":          r.command.priorOutcome(),
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize reconsideration evidence: %w", err)
	}
	return deltaJSON, nil
}

func (e *Engine) recordReconsideration(ctx context.Context, tx *sql.Tx, r reconsideration, correctionDigest []byte) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO reconsiderations (
			reconsideration_id, tenant_id, situation_id, superseded_version,
			correction_version, correction_snapshot_sha256, invalidated_command_id,
			invalidated_outcome_id, invalidated_outcome_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.reconsiderationID, e.tenantID, r.current.SituationID, r.current.PreviousVersion,
		r.current.Version, correctionDigest, r.command.commandID, r.command.outcomeID, r.command.outcomeSHA,
		e.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert reconsideration: %w", err)
	}
	return nil
}

// scheduleReconsideration saves an admitted deep-lane evaluation carrying the
// reconsideration evidence and inserts its pending scheduler item.
func (e *Engine) scheduleReconsideration(ctx context.Context, tx *sql.Tx, r reconsideration, deltaJSON []byte) error {
	eval := Evaluation{
		TriggerID: r.triggerID, TriggerName: reconsiderationTrigger,
		SituationID: r.current.SituationID, SituationVersion: r.current.Version,
		Score: 100, Threshold: 0, Lane: "deep", Outcome: "admitted",
		Reasons:      []string{"accepted action invalidated by corrected Situation version"},
		PolicySHA256: e.spec.Digest, DeltaJSON: deltaJSON, EvaluatedAt: e.clk.Now().UTC(),
	}
	if err := e.scheduler.saveEvaluation(ctx, tx, eval, e.tenantID, e.deploymentID); err != nil {
		return fmt.Errorf("save reconsideration evaluation: %w", err)
	}
	item := scheduleledger.Item{
		SchedulerItemID: r.schedulerItemID, Kind: "reconsider", TriggerID: r.triggerID,
		SituationID: r.current.SituationID, SituationVersion: r.current.Version,
		Lane: "deep", Priority: 100, Status: "pending",
		ExpiresAt: e.clk.Now().UTC().Add(defaultExpiresAfter),
	}
	if err := e.scheduler.insertItem(ctx, tx, item, e.tenantID); err != nil {
		return fmt.Errorf("insert reconsideration item: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reconsiderations SET trigger_id = ?, scheduler_item_id = ? WHERE reconsideration_id = ?`, r.triggerID, r.schedulerItemID, r.reconsiderationID); err != nil {
		return fmt.Errorf("link reconsideration admission: %w", err)
	}
	return nil
}

func (e *Engine) announceReconsideration(ctx context.Context, tx *sql.Tx, r reconsideration) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx,
		"reconsideration.admitted:"+r.reconsiderationID, e.tenantID, notify.TypeReconsiderationAdmitted,
		"situation/"+r.current.SituationID, r.current.SituationID, map[string]any{
			"tenant_id": e.tenantID, "reconsideration_id": r.reconsiderationID, "situation_id": r.current.SituationID,
			"superseded_version": r.current.PreviousVersion, "correction_version": r.current.Version,
			"invalidated_command_id": r.command.commandID, "invalidated_outcome_id": r.command.outcomeID,
			"trigger_id": r.triggerID, "scheduler_item_id": r.schedulerItemID,
			"source_authority": notify.SourceForTenant(e.tenantID),
		}, e.clk.Now().UTC(), contractsv1.TraceContext{Traceparent: r.current.Traceparent, Tracestate: r.current.Tracestate}); err != nil {
		return fmt.Errorf("append reconsideration notification: %w", err)
	}
	return nil
}
