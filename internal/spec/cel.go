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

	check := func(path, expr string) error {
		if expr == "" {
			return nil
		}
		_, issues := env.Compile(expr)
		if issues != nil && issues.Err() != nil {
			return &CompileError{Path: path, Message: fmt.Sprintf("cel: %v", issues.Err())}
		}
		return nil
	}

	if err := check("situation.occurrence.openWhen", spec.Situation.Occurrence.OpenWhen); err != nil {
		return err
	}
	if err := check("situation.occurrence.closeWhen", spec.Situation.Occurrence.CloseWhen); err != nil {
		return err
	}
	for i, t := range spec.Situation.Transitions {
		if err := check(fmt.Sprintf("situation.transitions[%d].when", i), t.When); err != nil {
			return err
		}
	}
	for i, tr := range spec.Cognition.Triggers {
		if err := check(fmt.Sprintf("cognition.triggers[%d].when", i), tr.When); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("cognition.triggers[%d].score", i), tr.Score); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("cognition.triggers[%d].materialDelta", i), tr.MaterialDelta); err != nil {
			return err
		}
	}
	for i, op := range spec.Operators {
		if err := check(fmt.Sprintf("operators[%d].where", i), op.Where); err != nil {
			return err
		}
	}

	return nil
}
