package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// RunShadow replays the request's trace and, for every replayed episode,
// pairs the deterministic baseline with the candidate worker on socketPath.
// Both see the same immutable snapshot; the comparison is sealed in the
// replay database and nothing is dispatched.
func RunShadow(ctx context.Context, request Request, socketPath, workerName string) (domain.Result, error) {
	baseline, err := compileBaseline(ctx, request.SpecPath)
	if err != nil {
		return domain.Result{}, err
	}
	candidate, err := transport.DialShadowWorker(ctx, socketPath, workerName)
	if err != nil {
		return domain.Result{}, err //nolint:wrapcheck // The transport names the failed step.
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
