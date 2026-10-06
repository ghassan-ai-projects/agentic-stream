package domain

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Rules) compilePrograms() error {
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

func (e *Rules) compileProgram(name, src string) (cel.Program, error) {
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

func (e *Rules) evalBool(tr spec.Trigger, kind string, in triggerInputs) (bool, error) {
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
func (e *Rules) evalProgram(name string, in triggerInputs) (ref.Val, error) {
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

func (e *Rules) evalScore(tr spec.Trigger, in triggerInputs) (float64, error) {
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
