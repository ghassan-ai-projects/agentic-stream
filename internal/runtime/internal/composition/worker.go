package composition

import (
	"context"
	"fmt"

	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
)

// NewWorkerRuntime validates configuration before allocating resource ownership.
func NewWorkerRuntime(ctx context.Context, cfg transport.WorkerRuntimeConfig) (*app.WorkerRuntime, error) {
	return newWorkerRuntime(ctx, cfg, nativeexecutor.New)
}

func newWorkerRuntime(ctx context.Context, cfg transport.WorkerRuntimeConfig, constructor transport.NativeConstructor) (*app.WorkerRuntime, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("worker runtime database is required")
	}
	if err := ValidateWorkerRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = "native"
	}
	return app.NewWorkerRuntime(ctx, transport.WorkerOptions(cfg), transport.NewWorkerBackend(cfg, constructor))
}

// ValidateWorkerRuntimeConfig checks only pure option consistency.
func ValidateWorkerRuntimeConfig(cfg transport.WorkerRuntimeConfig) error {
	return domain.ValidateWorkerOptions(transport.WorkerOptions(cfg))
}
