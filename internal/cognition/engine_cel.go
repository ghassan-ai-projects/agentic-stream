package cognition

import (
	"fmt"
	"reflect"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

func (e *Engine) compilePrograms() error {
	for _, tr := range e.spec.Cognition.Triggers {
		for _, expr := range triggerSources(tr) {
			if expr.src == "" {
				continue
			}
			prg, err := e.compileProgram(expr.name, expr.src)
			if err != nil {
				return err
			}
			e.programs[expr.name] = prg
		}
	}
	return nil
}

// triggerSource is one named trigger expression.
type triggerSource struct{ name, src string }

func triggerSources(tr spec.Trigger) []triggerSource {
	return []triggerSource{
		{tr.Name + ":when", tr.When},
		{tr.Name + ":score", tr.Score},
		{tr.Name + ":materialDelta", tr.MaterialDelta},
	}
}

func (e *Engine) compileProgram(name, src string) (cel.Program, error) {
	ast, issues := e.celEnv.Compile(src)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile %s: %w", name, issues.Err())
	}
	prg, err := e.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program %s: %w", name, err)
	}
	return prg, nil
}

func (e *Engine) buildFeatures(v situations.Version) map[string]any {
	evidence := v.Evidence
	if evidence == nil {
		evidence = []string{}
	}
	return situations.CELFeatures(e.spec, v.Facts, evidence)
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
		return firstVersionDelta(current)
	}
	prevFacts := previous.Facts
	if prevFacts == nil {
		prevFacts = map[string]any{}
	}
	return changeDelta(current, *previous, prevFacts)
}

// changeDelta describes what changed since the previously reasoned version.
func changeDelta(current, previous situations.Version, prevFacts map[string]any) map[string]any {
	factsChanged := !mapsEqual(prevFacts, current.Facts)
	phaseChanged := current.Phase != previous.Phase
	return map[string]any{
		spec.DeltaKeys.PhaseChanged: phaseChanged, spec.DeltaKeys.SeverityChange: current.Severity - previous.Severity,
		spec.DeltaKeys.CompletenessChanged: current.Completeness != previous.Completeness,
		// Primary-hypothesis tracking is not implemented in this slice; it is
		// intentionally false so triggers can reference the key deterministically.
		spec.DeltaKeys.PrimaryHypothesisChanged: false, spec.DeltaKeys.FactsChanged: factsChanged,
		spec.DeltaKeys.Facts: prevFacts, spec.DeltaKeys.NewFacts: current.Facts,
		spec.DeltaKeys.Novelty: novelty(phaseChanged || factsChanged),
	}
}

// firstVersionDelta treats every aspect of a first reasoned version as new.
func firstVersionDelta(current situations.Version) map[string]any {
	return map[string]any{
		spec.DeltaKeys.PhaseChanged: true, spec.DeltaKeys.SeverityChange: current.Severity,
		spec.DeltaKeys.CompletenessChanged: true, spec.DeltaKeys.PrimaryHypothesisChanged: true,
		spec.DeltaKeys.FactsChanged: true, spec.DeltaKeys.Facts: map[string]any{},
		spec.DeltaKeys.NewFacts: current.Facts, spec.DeltaKeys.Novelty: 1.0,
	}
}

func novelty(changed bool) float64 {
	if changed {
		return 1.0
	}
	return 0.0
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

func (e *Engine) evalBool(tr spec.Trigger, kind string, in triggerInputs) (bool, error) {
	if triggerExpression(tr, kind) == "" {
		return false, nil
	}
	out, err := e.evalProgram(tr.Name+":"+kind, in)
	if err != nil {
		return false, err
	}
	return spec.CELBool(out) //nolint:wrapcheck // The spec helper names the conversion failure.
}

// triggerExpression is the trigger's boolean expression of the given kind.
func triggerExpression(tr spec.Trigger, kind string) string {
	switch kind {
	case "when":
		return tr.When
	case "materialDelta":
		return tr.MaterialDelta
	default:
		return ""
	}
}

// evalProgram runs a precompiled trigger program over the trigger inputs.
func (e *Engine) evalProgram(name string, in triggerInputs) (ref.Val, error) {
	prg, ok := e.programs[name]
	if !ok {
		return nil, fmt.Errorf("no compiled program for %s", name)
	}
	out, _, err := prg.Eval(map[string]any{
		"features": in.features, "situation": in.situation, "delta": in.delta,
		"event_time": in.eventHorizon, "watermark": in.watermark,
	})
	if err != nil {
		return nil, fmt.Errorf("eval cel: %w", err)
	}
	return out, nil
}

func (e *Engine) evalScore(tr spec.Trigger, in triggerInputs) (float64, error) {
	if tr.Score == "" {
		return 0, nil
	}
	out, err := e.evalProgram(tr.Name+":score", in)
	if err != nil {
		return 0, err
	}
	return celNumber(out)
}

func celNumber(out ref.Val) (float64, error) {
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
