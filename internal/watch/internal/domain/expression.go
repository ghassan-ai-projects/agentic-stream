package domain

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

func ValidateExpression(expression string) error {
	if strings.ContainsAny(expression, "{};`") {
		return fmt.Errorf("watch expression contains forbidden syntax")
	}
	_, _, err := compile(expression)
	return err
}

func Evaluate(expression string, features map[string]any) (bool, error) {
	program, err := compiledProgram(expression)
	if err != nil {
		return false, err
	}
	return evaluateProgram(program, features)
}

var compiledPrograms sync.Map

func compiledProgram(expression string) (cel.Program, error) {
	if cached, ok := compiledPrograms.Load(expression); ok {
		return cached.(cel.Program), nil
	}
	env, ast, err := compile(expression)
	if err != nil {
		return nil, err
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("build watch expression program: %w", err)
	}
	compiledPrograms.Store(expression, program)
	return program, nil
}

var watchEnvironment = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)), cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)), ext.Bindings())
})

func compile(expression string) (*cel.Env, *cel.Ast, error) {
	env, err := watchEnvironment()
	if err != nil {
		return nil, nil, fmt.Errorf("create watch expression environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, nil, fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	return env, ast, nil
}

func evaluateProgram(program cel.Program, features map[string]any) (bool, error) {
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
