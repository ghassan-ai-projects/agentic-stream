// Package cognition implements the deterministic cognitive scheduler: trigger
// evaluation, scoring, and admission against a durable scheduler queue.
package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
		deploymentID: deploymentID,
		tenantID:     tenantID,
		spec:         compiled,
		celEnv:       env,
		scheduler:    NewScheduler(compiled, idGen, clk),
		idGen:        idGen,
		clk:          clk,
		programs:     make(map[string]cel.Program),
	}
	if err := eng.compilePrograms(); err != nil {
		return nil, fmt.Errorf("compile trigger programs: %w", err)
	}
	return eng, nil
}

// Process evaluates all triggers for a new Situation version and updates the
// durable scheduler queue. It runs inside the supplied transaction.
func (e *Engine) Process(ctx context.Context, tx *sql.Tx, v situations.Version) error {
	lastReasoned, err := e.loadLastReasonedVersion(ctx, tx, v.SituationID)
	if err != nil {
		return fmt.Errorf("load last reasoned version: %w", err)
	}

	previous, err := e.loadVersion(ctx, tx, v.SituationID, lastReasoned)
	if err != nil {
		return fmt.Errorf("load previous version: %w", err)
	}

	if err := e.evaluateTriggers(ctx, tx, v, previous); err != nil {
		return err
	}
	if _, err := e.admitReconsiderations(ctx, tx, v); err != nil {
		return fmt.Errorf("admit reconsideration: %w", err)
	}

	return e.markVersionReasoned(ctx, tx, v)
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
	var v situations.Version
	var eventHorizonStr, watermarkStr string
	var traceparent, tracestate sql.NullString
	var snapshotJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT sv.situation_id, sv.version, sv.phase, sv.previous_phase, sv.severity,
		       sv.confidence, sv.completeness, s.entity_type, s.entity_id,
		       sv.event_horizon, sv.watermark, sv.traceparent, sv.tracestate, sv.snapshot_json
		FROM situation_versions sv
		JOIN situations s ON s.situation_id = sv.situation_id
		WHERE sv.situation_id = ? AND sv.version = ?`,
		situationID, version,
	).Scan(
		&v.SituationID, &v.Version, &v.Phase, &v.PreviousPhase,
		&v.Severity, &v.Confidence, &v.Completeness,
		&v.EntityType, &v.EntityID, &eventHorizonStr, &watermarkStr, &traceparent, &tracestate, &snapshotJSON,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query version: %w", err)
	}
	eventHorizon, err := time.Parse(time.RFC3339Nano, eventHorizonStr)
	if err != nil {
		return nil, fmt.Errorf("parse event horizon: %w", err)
	}
	v.EventHorizon = eventHorizon
	if _, err := contractsv1.ParseTraceContext(traceparent.String, tracestate.String); err != nil {
		return nil, fmt.Errorf("validate version trace context: %w", err)
	}
	v.Traceparent = traceparent.String
	v.Tracestate = tracestate.String
	if watermarkStr != "" {
		watermark, err := time.Parse(time.RFC3339Nano, watermarkStr)
		if err != nil {
			return nil, fmt.Errorf("parse watermark: %w", err)
		}
		v.Watermark = watermark
	}
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if facts, ok := snapshot["facts"].(map[string]any); ok {
		v.Facts = facts
	}
	return &v, nil
}

func (e *Engine) triggerID(name, situationID string, version int) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|%s", e.deploymentID, situationID, version, name)
	return ids.PrefixTrigger + hex.EncodeToString(h.Sum(nil))[:24]
}
