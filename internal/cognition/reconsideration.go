package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	return e.admitInvalidated(ctx, tx, current, correction, correctionDigest, invalidated)
}

// admitInvalidated admits one reconsideration per invalidated command,
// counting only those not already admitted.
func (e *Engine) admitInvalidated(ctx context.Context, tx *sql.Tx, current situations.Version, correction map[string]any, correctionDigest []byte, invalidated []invalidatedCommand) (int, error) {
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

// invalidatedCommand is a succeeded command, with its latest outcome, whose
// authorizing Situation version the correction superseded.
type invalidatedCommand struct {
	commandID, decisionID, outcomeID, outcomeStatus, reconciliationStatus string
	outcomeOrdinal                                                        int
	providerJSON, observedJSON, outcomeSHA                                []byte
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
	if err := e.persistReconsideration(ctx, tx, r, correctionDigest, deltaJSON); err != nil {
		return false, err
	}
	return true, nil
}

// persistReconsideration records the reconsideration, schedules its episode
// and announces it, in that order.
func (e *Engine) persistReconsideration(ctx context.Context, tx *sql.Tx, r reconsideration, correctionDigest, deltaJSON []byte) error {
	if err := e.recordReconsideration(ctx, tx, r, correctionDigest); err != nil {
		return err
	}
	if err := e.scheduleReconsideration(ctx, tx, r, deltaJSON); err != nil {
		return err
	}
	return e.announceReconsideration(ctx, tx, r)
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
	if err := e.scheduler.saveEvaluation(ctx, tx, e.reconsiderationEvaluation(r, deltaJSON), e.tenantID, e.deploymentID); err != nil {
		return fmt.Errorf("save reconsideration evaluation: %w", err)
	}
	if err := e.scheduler.insertItem(ctx, tx, e.reconsiderationItem(r), e.tenantID); err != nil {
		return fmt.Errorf("insert reconsideration item: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reconsiderations SET trigger_id = ?, scheduler_item_id = ? WHERE reconsideration_id = ?`, r.triggerID, r.schedulerItemID, r.reconsiderationID); err != nil {
		return fmt.Errorf("link reconsideration admission: %w", err)
	}
	return nil
}

// reconsiderationEvaluation is the admitted deep-lane evaluation that
// explains why the reconsideration exists.
func (e *Engine) reconsiderationEvaluation(r reconsideration, deltaJSON []byte) Evaluation {
	return Evaluation{
		TriggerID: r.triggerID, TriggerName: reconsiderationTrigger,
		SituationID: r.current.SituationID, SituationVersion: r.current.Version,
		Score: 100, Threshold: 0, Lane: "deep", Outcome: "admitted",
		Reasons:      []string{"accepted action invalidated by corrected Situation version"},
		PolicySHA256: e.spec.Digest, DeltaJSON: deltaJSON, EvaluatedAt: e.clk.Now().UTC(),
	}
}

func (e *Engine) reconsiderationItem(r reconsideration) scheduleledger.Item {
	return scheduleledger.Item{
		SchedulerItemID: r.schedulerItemID, Kind: "reconsider", TriggerID: r.triggerID,
		SituationID: r.current.SituationID, SituationVersion: r.current.Version,
		Lane: "deep", Priority: 100, Status: "pending",
		ExpiresAt: e.clk.Now().UTC().Add(defaultExpiresAfter),
	}
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
