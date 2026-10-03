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
	_, _, err := compileWatchExpression(expression)
	return err
}

func evaluateWatchExpression(expression string, features map[string]any) (bool, error) {
	env, ast, err := compileWatchExpression(expression)
	if err != nil {
		return false, err
	}
	program, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("build watch expression program: %w", err)
	}
	return evaluateWatchProgram(program, features)
}

func compileWatchExpression(expression string) (*cel.Env, *cel.Ast, error) {
	env, err := cel.NewEnv(cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)), cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)), ext.Bindings())
	if err != nil {
		return nil, nil, fmt.Errorf("create watch expression environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, nil, fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	return env, ast, nil
}

func evaluateWatchProgram(program cel.Program, features map[string]any) (bool, error) {
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
