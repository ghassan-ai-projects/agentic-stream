// Package app orders the spec use cases: compile a SituationSpec from a file
// (read the file, then compile and seal it through the domain rules). Persisting
// deployments belongs to the store.
package app

import (
	"context"
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

type CompileOption func(*domain.Compiler)

func WithEventSchemas(schemas *domain.EventSchemas) CompileOption {
	return func(compiler *domain.Compiler) { compiler.WithEventSchemas(schemas) }
}

func CompileFile(ctx context.Context, path string, options ...CompileOption) (*domain.CompiledSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	compiler := domain.NewCompiler()
	for _, option := range options {
		option(compiler)
	}
	return compiler.CompileBytes(ctx, data, path) //nolint:wrapcheck // The compiler's CompileError carries the operator-facing path and message.
}
