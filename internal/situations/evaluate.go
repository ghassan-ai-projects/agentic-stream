package situations

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"reflect"
	"sort"
	"time"
)

func (e *Engine) evaluate(ctx context.Context, sit *Situation, feature operators.Feature, watermark time.Time, completenessChanged bool) (*Version, error) {
	features := e.buildFeaturesMap(sit)
	situation := e.buildSituationMap(sit)
	inputs := evaluationInputs{features: features, situation: situation, eventTime: feature.EventTime, watermark: watermark}

	// Check occurrence close first if active, then phase transitions, then
	// occurrence open if not active.
	closed, err := e.closeOccurrence(ctx, sit, inputs)
	if err != nil {
		return nil, err
	}
	transitioned, err := e.applyTransitions(ctx, sit, inputs)
	if err != nil {
		return nil, err
	}
	opened, err := e.openOccurrence(ctx, sit, inputs)
	if err != nil {
		return nil, err
	}
	changed := closed || transitioned || opened
	if completenessChanged && sit.Version > 0 && !changed {
		sit.Version++
		sit.UpdatedAt = watermark
		changed = true
	}
	if !changed {
		return nil, nil
	}
	return e.materialize(sit, watermark)
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
		cond, err := e.evalBool(ctx, tr.When, in.features, in.situation)
		if err != nil {
			return false, err
		}
		key := tr.From + "->" + tr.To
		if !cond {
			delete(sit.ConditionStart, key)
			continue
		}
		start := sit.ConditionStart[key]
		if start.IsZero() {
			start = in.eventTime
			sit.ConditionStart[key] = start
		}
		minDur, _ := duration.Parse(tr.MinDuration)
		if in.eventTime.Sub(start) >= minDur && e.transition(sit, tr.To, in.watermark) {
			changed = true
		}
	}
	return changed, nil
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
	features := make(map[string]any)
	for _, r := range e.spec.Situation.Reducers {
		switch r.Strategy {
		case "latest_event_time":
			// A nil fact means this operator has not materialized an output yet.
			// Leave it absent so the typed operator default below remains effective.
			if v, ok := sit.Facts[r.Field]; ok && v != nil {
				features[r.Input] = v
			}
		case "set_union":
			evidence := make([]string, 0, len(sit.Evidence))
			for id := range sit.Evidence {
				evidence = append(evidence, id)
			}
			sort.Strings(evidence)
			features[r.Field] = evidence
		}
	}
	// Pre-populate defaults for every operator output so that CEL expressions
	// never fail on a missing key. Numeric features default to 0; heartbeat
	// detectors default to false.
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

func (e *Engine) evalBool(ctx context.Context, expr string, features, situation map[string]any) (bool, error) {
	_ = ctx
	if expr == "" {
		return false, nil
	}
	ast, issues := e.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("compile cel: %w", issues.Err())
	}
	prg, err := e.celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("program cel: %w", err)
	}
	out, _, err := prg.Eval(map[string]any{
		"features":  features,
		"situation": situation,
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
