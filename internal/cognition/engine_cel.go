package cognition

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

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

func (e *Engine) buildFeatures(v situations.Version) map[string]any {
	features := make(map[string]any)
	for _, r := range e.spec.Situation.Reducers {
		switch r.Strategy {
		case "latest_event_time":
			// A nil fact means this operator has not materialized an output yet.
			// Leave it absent so the typed operator default below remains effective.
			if val, ok := v.Facts[r.Field]; ok && val != nil {
				features[r.Input] = val
			}
		case "set_union":
			evidence := v.Evidence
			if evidence == nil {
				evidence = []string{}
			}
			features[r.Field] = evidence
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
