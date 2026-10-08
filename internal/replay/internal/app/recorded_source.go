package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func RunRecorded(ctx context.Context, request Request, sourcePath string) (domain.Result, error) {
	compiled, err := spec.CompileFile(ctx, request.SpecPath)
	if err != nil {
		return domain.Result{}, fmt.Errorf("compile spec: %w", err)
	}
	source, err := transport.OpenSourceDatabase(ctx, sourcePath)
	if err != nil {
		return domain.Result{}, err
	}
	defer func() { _ = source.Close() }()
	ledger := store.NewSourceLedger(source.DB, request.TenantID, compiled.Digest)
	if err := ledger.RequireDeployment(ctx); err != nil {
		return domain.Result{}, err
	}
	return RunMode(ctx, domain.ModeRecorded, request, domain.Capabilities{RecordedLedger: ledger})
}
