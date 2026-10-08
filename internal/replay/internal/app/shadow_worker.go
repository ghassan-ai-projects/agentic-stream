package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func RunShadow(ctx context.Context, request Request, socketPath, workerName string) (domain.Result, error) {
	baseline, err := compileBaseline(ctx, request.SpecPath)
	if err != nil {
		return domain.Result{}, err
	}
	candidate, err := transport.DialShadowWorker(ctx, socketPath, workerName)
	if err != nil {
		return domain.Result{}, fmt.Errorf("connect shadow candidate %s: %w", socketPath, err)
	}
	defer func() { _ = candidate.Close() }()
	return RunMode(ctx, domain.ModeShadow, request, domain.Capabilities{BaselineExecutor: baseline, ShadowExecutor: candidate})
}

func compileBaseline(ctx context.Context, specPath string) (*domain.BaselinePolicy, error) {
	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return nil, fmt.Errorf("compile spec: %w", err)
	}
	return NewDeterministicBaseline(compiled)
}
