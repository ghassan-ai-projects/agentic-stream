package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// RunRecorded replays the request's trace and verifies it against the
// decisions a live runtime recorded in the database at sourcePath: every
// replayed episode must have exactly one accepted decision citing the same
// situation version and snapshot. The source is opened read-only.
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
