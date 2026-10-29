package domain

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
)

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

type celExpression struct {
	path, source string
}

func specExpressions(spec *CompiledSpec) []celExpression {
	expressions := []celExpression{
		{"situation.occurrence.openWhen", spec.Situation.Occurrence.OpenWhen},
		{"situation.occurrence.closeWhen", spec.Situation.Occurrence.CloseWhen},
	}
	for i, t := range spec.Situation.Transitions {
		expressions = append(expressions, celExpression{fmt.Sprintf("situation.transitions[%d].when", i), t.When})
	}
	expressions = append(expressions, triggerExpressions(spec.Cognition.Triggers)...)
	for i, op := range spec.Operators {
		expressions = append(expressions, celExpression{fmt.Sprintf("operators[%d].where", i), op.Where})
	}
	return expressions
}

func triggerExpressions(triggers []Trigger) []celExpression {
	expressions := make([]celExpression, 0, 3*len(triggers))
	for i, tr := range triggers {
		expressions = append(expressions,
			celExpression{fmt.Sprintf("cognition.triggers[%d].when", i), tr.When},
			celExpression{fmt.Sprintf("cognition.triggers[%d].score", i), tr.Score},
			celExpression{fmt.Sprintf("cognition.triggers[%d].materialDelta", i), tr.MaterialDelta},
		)
	}
	return expressions
}

func CELBool(out ref.Val) (bool, error) {
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
