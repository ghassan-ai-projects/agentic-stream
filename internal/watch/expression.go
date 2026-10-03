package watch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

func validateWatchExpression(expression string) error {
	if strings.ContainsAny(expression, "{};`") {
		return fmt.Errorf("watch expression contains forbidden syntax")
	}
	env, err := cel.NewEnv(
		cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)),
		ext.Bindings(),
	)
	if err != nil {
		return fmt.Errorf("create watch expression environment: %w", err)
	}
	_, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	return nil
}

func evaluateWatchExpression(expression string, features map[string]any) (bool, error) {
	env, err := cel.NewEnv(
		cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)),
		ext.Bindings(),
	)
	if err != nil {
		return false, fmt.Errorf("create watch expression environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("build watch expression program: %w", err)
	}
	value, _, err := program.Eval(map[string]any{"features": features, "situation": map[string]any{}})
	if err != nil {
		return false, fmt.Errorf("evaluate watch expression: %w", err)
	}
	matched, ok := value.Value().(bool)
	if !ok {
		return false, fmt.Errorf("watch expression must return bool")
	}
	return matched, nil
}

func integerPayload(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int(number))
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
