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
	"reflect"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
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

func (e *Engine) compilePrograms() error {
	for _, tr := range e.spec.Cognition.Triggers {
		for _, expr := range []struct {
			name string
			src  string
		}{
			{tr.Name + ":when", tr.When},
			{tr.Name + ":score", tr.Score},
			{tr.Name + ":materialDelta", tr.MaterialDelta},
		} {
			if expr.src == "" {
				continue
			}
			ast, issues := e.celEnv.Compile(expr.src)
			if issues != nil && issues.Err() != nil {
				return fmt.Errorf("compile %s: %w", expr.name, issues.Err())
			}
			prg, err := e.celEnv.Program(ast)
			if err != nil {
				return fmt.Errorf("program %s: %w", expr.name, err)
			}
			e.programs[expr.name] = prg
		}
	}
	return nil
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

	for _, tr := range e.spec.Cognition.Triggers {
		eval, err := e.evaluate(ctx, tr, v, previous)
		if err != nil {
			return fmt.Errorf("evaluate trigger %s: %w", tr.Name, err)
		}
		if err := e.scheduler.Admit(ctx, tx, eval, v, e.tenantID, e.deploymentID); err != nil {
			return fmt.Errorf("admit trigger %s: %w", tr.Name, err)
		}
	}

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
	var snapshotJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT sv.situation_id, sv.version, sv.phase, sv.previous_phase, sv.severity,
		       sv.confidence, sv.completeness, s.entity_type, s.entity_id,
		       sv.event_horizon, sv.watermark, sv.snapshot_json
		FROM situation_versions sv
		JOIN situations s ON s.situation_id = sv.situation_id
		WHERE sv.situation_id = ? AND sv.version = ?`,
		situationID, version,
	).Scan(
		&v.SituationID, &v.Version, &v.Phase, &v.PreviousPhase,
		&v.Severity, &v.Confidence, &v.Completeness,
		&v.EntityType, &v.EntityID, &eventHorizonStr, &watermarkStr, &snapshotJSON,
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

func (e *Engine) evaluate(ctx context.Context, tr spec.Trigger, current situations.Version, previous *situations.Version) (Evaluation, error) {
	evalAt := e.clk.Now().UTC()
	eval := Evaluation{
		TriggerID:        e.triggerID(tr.Name, current.SituationID, current.Version),
		TriggerName:      tr.Name,
		SituationID:      current.SituationID,
		SituationVersion: current.Version,
		Threshold:        tr.Threshold,
		Lane:             tr.Lane,
		Outcome:          "ignored",
		PolicySHA256:     e.spec.Digest,
		EvaluatedAt:      evalAt,
	}

	features := e.buildFeatures(current)
	situation := e.buildSituation(current)
	delta := e.buildDelta(current, previous)
	deltaJSON, err := canonicaljson.Marshal(delta)
	if err != nil {
		return eval, fmt.Errorf("marshal delta: %w", err)
	}
	eval.DeltaJSON = deltaJSON

	fired, err := e.evalBool(ctx, tr, "when", features, situation, delta, current.EventHorizon, current.Watermark)
	if err != nil {
		return eval, err
	}
	if !fired {
		eval.Reasons = append(eval.Reasons, "trigger condition false")
		return eval, nil
	}

	score, err := e.evalScore(ctx, tr, features, situation, delta, current.EventHorizon, current.Watermark)
	if err != nil {
		return eval, err
	}
	eval.Score = score

	material := true
	if tr.MaterialDelta != "" {
		var err error
		material, err = e.evalBool(ctx, tr, "materialDelta", features, situation, delta, current.EventHorizon, current.Watermark)
		if err != nil {
			return eval, err
		}
	}
	if !material {
		eval.Reasons = append(eval.Reasons, "material delta false")
		return eval, nil
	}

	if score < tr.Threshold {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("score %.2f below threshold %.2f", score, tr.Threshold))
		return eval, nil
	}

	eval.Outcome = "admitted"
	eval.Reasons = append(eval.Reasons, fmt.Sprintf("score %.2f meets threshold %.2f", score, tr.Threshold))
	return eval, nil
}

func (e *Engine) triggerID(name, situationID string, version int) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|%s", e.deploymentID, situationID, version, name)
	return ids.PrefixTrigger + hex.EncodeToString(h.Sum(nil))[:24]
}

func (e *Engine) buildFeatures(v situations.Version) map[string]any {
	features := make(map[string]any)
	for _, r := range e.spec.Situation.Reducers {
		switch r.Strategy {
		case "latest_event_time":
			if val, ok := v.Facts[r.Field]; ok {
				features[r.Input] = val
			}
		case "set_union":
			features[r.Field] = v.Evidence
		}
	}
	// Pre-populate defaults for every operator output so CEL expressions never
	// fail on a missing key. Numeric features default to 0; heartbeat detectors
	// default to false.
	for _, op := range e.spec.Operators {
		if _, ok := features[op.Output]; ok {
			continue
		}
		switch op.Kind {
		case "missing_heartbeat":
			features[op.Output] = false
		default:
			features[op.Output] = 0.0
		}
	}
	return features
}

func (e *Engine) buildSituation(v situations.Version) map[string]any {
	return map[string]any{
		"phase":       v.Phase,
		"severity":    v.Severity,
		"confidence":  v.Confidence,
		"uncertainty": 1.0 - v.Confidence,
		"entity": map[string]any{
			"type": v.EntityType,
			"id":   v.EntityID,
		},
	}
}

func (e *Engine) buildDelta(current situations.Version, previous *situations.Version) map[string]any {
	if previous == nil {
		return map[string]any{
			spec.DeltaKeys.PhaseChanged:             true,
			spec.DeltaKeys.SeverityChange:           current.Severity,
			spec.DeltaKeys.CompletenessChanged:      true,
			spec.DeltaKeys.PrimaryHypothesisChanged: true,
			spec.DeltaKeys.FactsChanged:             true,
			spec.DeltaKeys.Facts:                    map[string]any{},
			spec.DeltaKeys.NewFacts:                 current.Facts,
			spec.DeltaKeys.Novelty:                  1.0,
		}
	}

	prevFacts := previous.Facts
	if prevFacts == nil {
		prevFacts = map[string]any{}
	}
	factsChanged := !mapsEqual(prevFacts, current.Facts)
	// Primary-hypothesis tracking is not implemented in this slice; it is
	// intentionally false so triggers can reference the key deterministically.
	primaryHypothesisChanged := false
	novelty := 0.0
	if current.Phase != previous.Phase || factsChanged {
		novelty = 1.0
	}

	return map[string]any{
		spec.DeltaKeys.PhaseChanged:             current.Phase != previous.Phase,
		spec.DeltaKeys.SeverityChange:           current.Severity - previous.Severity,
		spec.DeltaKeys.CompletenessChanged:      current.Completeness != previous.Completeness,
		spec.DeltaKeys.PrimaryHypothesisChanged: primaryHypothesisChanged,
		spec.DeltaKeys.FactsChanged:             factsChanged,
		spec.DeltaKeys.Facts:                    prevFacts,
		spec.DeltaKeys.NewFacts:                 current.Facts,
		spec.DeltaKeys.Novelty:                  novelty,
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(va, vb) {
			return false
		}
	}
	return true
}

func (e *Engine) evalBool(ctx context.Context, tr spec.Trigger, kind string, features, situation, delta map[string]any, eventTime, watermark time.Time) (bool, error) {
	_ = ctx
	expr := ""
	switch kind {
	case "when":
		expr = tr.When
	case "materialDelta":
		expr = tr.MaterialDelta
	}
	if expr == "" {
		return false, nil
	}
	prg, ok := e.programs[tr.Name+":"+kind]
	if !ok {
		return false, fmt.Errorf("no compiled program for %s:%s", tr.Name, kind)
	}
	out, _, err := prg.Eval(map[string]any{
		"features":   features,
		"situation":  situation,
		"delta":      delta,
		"event_time": eventTime,
		"watermark":  watermark,
	})
	if err != nil {
		return false, fmt.Errorf("eval cel: %w", err)
	}
	v, err := out.ConvertToNative(reflect.TypeOf(true))
	if err != nil {
		return false, fmt.Errorf("cel result not bool: %w", err)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("cel result not bool: %T", v)
	}
	return b, nil
}

func (e *Engine) evalScore(ctx context.Context, tr spec.Trigger, features, situation, delta map[string]any, eventTime, watermark time.Time) (float64, error) {
	if tr.Score == "" {
		return 0, nil
	}
	prg, ok := e.programs[tr.Name+":score"]
	if !ok {
		return 0, fmt.Errorf("no compiled program for %s:score", tr.Name)
	}
	out, _, err := prg.Eval(map[string]any{
		"features":   features,
		"situation":  situation,
		"delta":      delta,
		"event_time": eventTime,
		"watermark":  watermark,
	})
	if err != nil {
		return 0, fmt.Errorf("eval cel: %w", err)
	}
	switch x := out.Value().(type) {
	case float64:
		return x, nil
	case int64:
		return float64(x), nil
	case int:
		return float64(x), nil
	default:
		return 0, fmt.Errorf("cel result not number: %T", out.Value())
	}
}
