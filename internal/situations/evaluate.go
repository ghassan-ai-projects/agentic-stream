package situations

import (
	"context"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Engine) evaluate(ctx context.Context, sit *Situation, feature operators.Feature, watermark time.Time, completenessChanged bool) (*Version, error) {
	inputs := evaluationInputs{features: e.buildFeaturesMap(sit), situation: e.buildSituationMap(sit), eventTime: feature.EventTime, watermark: watermark}
	changed, err := e.advanceLifecycle(ctx, sit, inputs)
	if err != nil {
		return nil, err
	}
	if !changed && completenessChanged && sit.Version > 0 {
		sit.Version++
		sit.UpdatedAt = watermark
		changed = true
	}
	if !changed {
		return nil, nil
	}
	return e.materialize(sit, watermark)
}

// advanceLifecycle checks occurrence close first if active, then phase
// transitions, then occurrence open if not active.
func (e *Engine) advanceLifecycle(ctx context.Context, sit *Situation, inputs evaluationInputs) (bool, error) {
	closed, err := e.closeOccurrence(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	transitioned, err := e.applyTransitions(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	opened, err := e.openOccurrence(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	return closed || transitioned || opened, nil
}

// evaluationInputs are the CEL inputs and times one evaluation uses.
type evaluationInputs struct {
	features, situation map[string]any
	eventTime           time.Time
	watermark           time.Time
}

func (e *Engine) closeOccurrence(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	if sit.Version == 0 && sit.Phase == e.spec.Situation.InitialPhase {
		return false, nil
	}
	closed, err := e.evalBool(ctx, e.spec.Situation.Occurrence.CloseWhen, in.features, in.situation)
	if err != nil || !closed {
		return false, err
	}
	return e.transition(sit, "resolved", in.watermark), nil
}

// applyTransitions takes every transition out of the current phase whose
// condition has held for its minimum duration, measured in event time.
func (e *Engine) applyTransitions(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	changed := false
	for _, tr := range e.spec.Situation.Transitions {
		if tr.From != sit.Phase {
			continue
		}
		moved, err := e.applyTransition(ctx, sit, tr, in)
		if err != nil {
			return false, err
		}
		changed = changed || moved
	}
	return changed, nil
}

// applyTransition starts or clears the transition's condition timer and
// moves the Situation once the condition has held for the minimum duration.
func (e *Engine) applyTransition(ctx context.Context, sit *Situation, tr spec.Transition, in evaluationInputs) (bool, error) {
	held, err := e.evalBool(ctx, tr.When, in.features, in.situation)
	if err != nil {
		return false, err
	}
	key := tr.From + "->" + tr.To
	if !held {
		delete(sit.ConditionStart, key)
		return false, nil
	}
	start := conditionStart(sit, key, in.eventTime)
	minDur, _ := spec.ParseDuration(tr.MinDuration)
	return in.eventTime.Sub(start) >= minDur && e.transition(sit, tr.To, in.watermark), nil
}

// conditionStart returns when the keyed condition began to hold, recording
// eventTime when it starts now.
func conditionStart(sit *Situation, key string, eventTime time.Time) time.Time {
	start := sit.ConditionStart[key]
	if start.IsZero() {
		start = eventTime
		sit.ConditionStart[key] = start
	}
	return start
}

func (e *Engine) openOccurrence(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	if sit.Version != 0 || sit.Phase != e.spec.Situation.InitialPhase {
		return false, nil
	}
	opened, err := e.evalBool(ctx, e.spec.Situation.Occurrence.OpenWhen, in.features, in.situation)
	if err != nil || !opened {
		return false, err
	}
	sit.Version++
	return true, nil
}

func (e *Engine) transition(sit *Situation, to string, watermark time.Time) bool {
	if sit.Phase == to {
		return false
	}
	sit.PreviousPhase = sit.Phase
	sit.Phase = to
	sit.Version++
	sit.Severity = e.severityForPhase(to)
	sit.UpdatedAt = watermark
	sit.ConditionStart = make(map[string]time.Time)
	return true
}

func (e *Engine) severityForPhase(phase string) int {
	for _, p := range e.spec.Situation.Phases {
		if p.Name == phase {
			return p.Severity
		}
	}
	return 0
}

func (e *Engine) buildFeaturesMap(sit *Situation) map[string]any {
	return CELFeatures(e.spec, sit.Facts, sortedEvidenceIDs(sit))
}

// CELFeatures is the CEL `features` view of a Situation: its reduced facts
// and evidence, with every missing operator output defaulted so expressions
// never fail on a missing key. Situations and cognition share this one view.
func CELFeatures(compiled *spec.CompiledSpec, facts map[string]any, evidence []string) map[string]any {
	features := make(map[string]any)
	for _, r := range compiled.Situation.Reducers {
		addReducedFeature(features, facts, evidence, r)
	}
	addOperatorDefaults(features, compiled.Operators)
	return features
}

func addReducedFeature(features, facts map[string]any, evidence []string, r spec.Reducer) {
	switch r.Strategy {
	case "latest_event_time":
		// A nil fact means this operator has not materialized an output yet.
		// Leave it absent so the typed operator default remains effective.
		if v, ok := facts[r.Field]; ok && v != nil {
			features[r.Input] = v
		}
	case "set_union":
		features[r.Field] = evidence
	}
}

// addOperatorDefaults pre-populates every missing operator output so that CEL
// expressions never fail on a missing key: heartbeat detectors default to
// false and numeric features to 0.
func addOperatorDefaults(features map[string]any, ops []spec.Operator) {
	for _, op := range ops {
		if _, ok := features[op.Output]; ok {
			continue
		}
		if op.Kind == "missing_heartbeat" {
			features[op.Output] = false
		} else {
			features[op.Output] = 0.0
		}
	}
}

func (e *Engine) buildSituationMap(sit *Situation) map[string]any {
	return map[string]any{
		"phase":      sit.Phase,
		"severity":   sit.Severity,
		"confidence": sit.Confidence,
		"entity": map[string]any{
			"type": sit.EntityType,
			"id":   sit.EntityID,
		},
	}
}

func (e *Engine) evalBool(_ context.Context, expr string, features, situation map[string]any) (bool, error) {
	if expr == "" {
		return false, nil
	}
	prg, err := e.program(expr)
	if err != nil {
		return false, err
	}
	out, _, err := prg.Eval(map[string]any{"features": features, "situation": situation})
	if err != nil {
		return false, fmt.Errorf("eval cel: %w", err)
	}
	return spec.CELBool(out) //nolint:wrapcheck // The spec helper names the conversion failure.
}

func (e *Engine) program(expr string) (cel.Program, error) {
	ast, issues := e.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile cel: %w", issues.Err())
	}
	prg, err := e.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program cel: %w", err)
	}
	return prg, nil
}
