// Package cognition implements the deterministic cognitive scheduler: trigger
// evaluation, scoring, and admission against a durable scheduler queue.
package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Engine evaluates cognition triggers and admits scheduler items.
type Engine struct {
	deploymentID string
	tenantID     string
	spec         *spec.CompiledSpec
	celEnv       *cel.Env
	scheduler    *Scheduler
	idGen        ids.Generator
	clk          clock.Clock
	programs     map[string]cel.Program
}

// Evaluation is the deterministic result of evaluating one trigger.
type Evaluation struct {
	TriggerID        string
	TriggerName      string
	SituationID      string
	SituationVersion int
	Score            float64
	Threshold        float64
	Lane             string
	Outcome          string
	Reasons          []string
	PolicySHA256     string
	DeltaJSON        []byte
	EvaluatedAt      time.Time
}

// NewEngine creates a cognition engine.
func NewEngine(db *storage.DB, deploymentID, tenantID string, compiled *spec.CompiledSpec, idGen ids.Generator, clk clock.Clock) (*Engine, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	if clk == nil {
		clk = clock.Physical()
	}
	eng := &Engine{
		deploymentID: deploymentID, tenantID: tenantID, spec: compiled, celEnv: env,
		scheduler: NewScheduler(compiled, idGen, clk), idGen: idGen, clk: clk, programs: make(map[string]cel.Program),
	}
	if err := eng.compilePrograms(); err != nil {
		return nil, fmt.Errorf("compile trigger programs: %w", err)
	}
	return eng, nil
}

// Process evaluates all triggers for a new Situation version and updates the
// durable scheduler queue. It runs inside the supplied transaction.
func (e *Engine) Process(ctx context.Context, tx *sql.Tx, v situations.Version) error {
	previous, err := e.lastReasonedVersion(ctx, tx, v.SituationID)
	if err != nil {
		return err
	}
	if err := e.evaluateTriggers(ctx, tx, v, previous); err != nil {
		return err
	}
	if _, err := e.admitReconsiderations(ctx, tx, v); err != nil {
		return fmt.Errorf("admit reconsideration: %w", err)
	}
	return e.markVersionReasoned(ctx, tx, v)
}

// lastReasonedVersion loads the version cognition last reasoned about, which
// is nil before the first one.
func (e *Engine) lastReasonedVersion(ctx context.Context, tx *sql.Tx, situationID string) (*situations.Version, error) {
	lastReasoned, err := e.loadLastReasonedVersion(ctx, tx, situationID)
	if err != nil {
		return nil, fmt.Errorf("load last reasoned version: %w", err)
	}
	previous, err := e.loadVersion(ctx, tx, situationID, lastReasoned)
	if err != nil {
		return nil, fmt.Errorf("load previous version: %w", err)
	}
	return previous, nil
}

func (e *Engine) evaluateTriggers(ctx context.Context, tx *sql.Tx, v situations.Version, previous *situations.Version) error {
	for _, tr := range e.spec.Cognition.Triggers {
		eval, err := e.evaluate(ctx, tr, v, previous)
		if err != nil {
			return fmt.Errorf("evaluate trigger %s: %w", tr.Name, err)
		}
		if err := e.scheduler.Admit(ctx, tx, eval, v, e.tenantID, e.deploymentID); err != nil {
			return fmt.Errorf("admit trigger %s: %w", tr.Name, err)
		}
	}
	return nil
}

func (e *Engine) markVersionReasoned(ctx context.Context, tx *sql.Tx, v situations.Version) error {
	// Advance last_reasoned_version unconditionally so that future deltas compare
	// against the most recently evaluated version regardless of outcome.
	if _, err := tx.ExecContext(ctx,
		"UPDATE situations SET last_reasoned_version = ? WHERE situation_id = ?",
		v.Version, v.SituationID,
	); err != nil {
		return fmt.Errorf("update last reasoned version: %w", err)
	}

	return nil
}

func (e *Engine) loadLastReasonedVersion(ctx context.Context, tx *sql.Tx, situationID string) (int, error) {
	var last int
	if err := tx.QueryRowContext(ctx,
		"SELECT last_reasoned_version FROM situations WHERE situation_id = ?",
		situationID,
	).Scan(&last); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("query last reasoned version: %w", err)
	}
	return last, nil
}

func (e *Engine) loadVersion(ctx context.Context, tx *sql.Tx, situationID string, version int) (*situations.Version, error) {
	if version <= 0 {
		return nil, nil
	}
	row, found, err := queryVersionRow(ctx, tx, situationID, version)
	if err != nil || !found {
		return nil, err
	}
	return row.version()
}

// versionRow is one stored Situation version with its entity, as scanned.
type versionRow struct {
	v                       situations.Version
	eventHorizon, watermark string
	traceparent, tracestate sql.NullString
	snapshotJSON            []byte
}

func queryVersionRow(ctx context.Context, tx *sql.Tx, situationID string, version int) (versionRow, bool, error) {
	var r versionRow
	err := tx.QueryRowContext(ctx, selectVersionSQL, situationID, version).Scan(
		&r.v.SituationID, &r.v.Version, &r.v.Phase, &r.v.PreviousPhase,
		&r.v.Severity, &r.v.Confidence, &r.v.Completeness,
		&r.v.EntityType, &r.v.EntityID, &r.eventHorizon, &r.watermark, &r.traceparent, &r.tracestate, &r.snapshotJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return versionRow{}, false, nil
	}
	if err != nil {
		return versionRow{}, false, fmt.Errorf("query version: %w", err)
	}
	return r, true, nil
}

const selectVersionSQL = `
		SELECT sv.situation_id, sv.version, sv.phase, sv.previous_phase, sv.severity,
		       sv.confidence, sv.completeness, s.entity_type, s.entity_id,
		       sv.event_horizon, sv.watermark, sv.traceparent, sv.tracestate, sv.snapshot_json
		FROM situation_versions sv
		JOIN situations s ON s.situation_id = sv.situation_id
		WHERE sv.situation_id = ? AND sv.version = ?`

// version parses the row's times, validates its trace context and recovers
// the snapshot facts.
func (r versionRow) version() (*situations.Version, error) {
	v := r.v
	var err error
	if v.EventHorizon, err = time.Parse(time.RFC3339Nano, r.eventHorizon); err != nil {
		return nil, fmt.Errorf("parse event horizon: %w", err)
	}
	if v.Traceparent, v.Tracestate, err = r.traceContext(); err != nil {
		return nil, err
	}
	if v.Watermark, err = parseOptionalTime(r.watermark); err != nil {
		return nil, fmt.Errorf("parse watermark: %w", err)
	}
	if v.Facts, err = snapshotFacts(r.snapshotJSON); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r versionRow) traceContext() (string, string, error) {
	if _, err := contractsv1.ParseTraceContext(r.traceparent.String, r.tracestate.String); err != nil {
		return "", "", fmt.Errorf("validate version trace context: %w", err)
	}
	return r.traceparent.String, r.tracestate.String, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value) //nolint:wrapcheck // The caller names the field.
}

// snapshotFacts returns the snapshot's facts object, or nil when it has none.
func snapshotFacts(snapshotJSON []byte) (map[string]any, error) {
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	facts, _ := snapshot["facts"].(map[string]any)
	return facts, nil
}

func (e *Engine) triggerID(name, situationID string, version int) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|%s", e.deploymentID, situationID, version, name)
	return ids.PrefixTrigger + hex.EncodeToString(h.Sum(nil))[:24]
}
