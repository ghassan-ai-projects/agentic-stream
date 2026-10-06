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

// CompileFile reads a spec from path and compiles it.
func CompileFile(ctx context.Context, path string) (*domain.CompiledSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return domain.NewCompiler().CompileBytes(ctx, data, path) //nolint:wrapcheck // The compiler's CompileError carries the operator-facing path and message.
}
