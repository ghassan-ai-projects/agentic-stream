package runtime

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	composition "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/composition"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
)

// WorkerRuntimeConfig is the complete native/remote connection configuration.
type WorkerRuntimeConfig = transport.WorkerRuntimeConfig

// WorkerRuntime exposes the selected Executor and delegates resource lifetimes.
type WorkerRuntime struct {
	Executor    episodes.Executor
	application *app.WorkerRuntime
}

// NewWorkerRuntime constructs only the configured native or remote route.
func NewWorkerRuntime(ctx context.Context, cfg WorkerRuntimeConfig) (*WorkerRuntime, error) {
	application, err := composition.NewWorkerRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &WorkerRuntime{Executor: application.Executor, application: application}, nil
}
func (r *WorkerRuntime) useCases() *app.WorkerRuntime {
	if r == nil {
		return nil
	}
	return r.application
}

// Errors reports asynchronous evidence-server failures.
func (r *WorkerRuntime) Errors() <-chan error { return r.useCases().Errors() }

// Close releases worker/evidence resources, including partial initialization.
func (r *WorkerRuntime) Close() error { return r.useCases().Close() }
