package spec

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// NewCELEnv creates a restricted CEL environment for SituationSpec expressions.
// It allows only deterministic functions and rejects sources of non-determinism
// such as timestamps, randomness, and external calls.
func NewCELEnv() (*cel.Env, error) {
	opts := []cel.EnvOption{
		cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("delta", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("event_time", cel.TimestampType),
		cel.Variable("watermark", cel.TimestampType),
		ext.Bindings(),
	}
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("create cel env: %w", err)
	}
	return env, nil
}

// validateExpressions compiles every CEL expression in the spec and reports the
// first error.
func validateExpressions(spec *CompiledSpec) error {
	env, err := NewCELEnv()
	if err != nil {
		return err
	}
	for _, expression := range specExpressions(spec) {
		if expression.source == "" {
			continue
		}
		if _, issues := env.Compile(expression.source); issues != nil && issues.Err() != nil {
			return &CompileError{Path: expression.path, Message: fmt.Sprintf("cel: %v", issues.Err())}
		}
	}
	return nil
}

// celExpression is one CEL source in the spec and the path that names it in
// a compile error.
type celExpression struct {
	path, source string
}

// specExpressions lists every CEL expression in the spec, in the order errors
// are reported.
func specExpressions(spec *CompiledSpec) []celExpression {
	expressions := []celExpression{
		{"situation.occurrence.openWhen", spec.Situation.Occurrence.OpenWhen},
		{"situation.occurrence.closeWhen", spec.Situation.Occurrence.CloseWhen},
	}
	for i, t := range spec.Situation.Transitions {
		expressions = append(expressions, celExpression{fmt.Sprintf("situation.transitions[%d].when", i), t.When})
	}
	for i, tr := range spec.Cognition.Triggers {
		expressions = append(expressions,
			celExpression{fmt.Sprintf("cognition.triggers[%d].when", i), tr.When},
			celExpression{fmt.Sprintf("cognition.triggers[%d].score", i), tr.Score},
			celExpression{fmt.Sprintf("cognition.triggers[%d].materialDelta", i), tr.MaterialDelta},
		)
	}
	for i, op := range spec.Operators {
		expressions = append(expressions, celExpression{fmt.Sprintf("operators[%d].where", i), op.Where})
	}
	return expressions
}
