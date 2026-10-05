package replay

import (
	"context"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// Request identifies one replay session: the isolated database path, the
// spec file, the trace file and the tenant.
type Request = domain.Request

// Run replays the request's trace against its spec and returns the canonical
// result.
func Run(ctx context.Context, request Request) (Result, error) {
	return app.Run(ctx, request)
}

// RunMode executes a replay mode without accepting credentials, effectors, or
// a resolver. Only counterfactual simulation may be added at a higher layer.
func RunMode(ctx context.Context, mode Mode, request Request, capabilities ...Capabilities) (Result, error) {
	return app.RunMode(ctx, mode, request, capabilities...)
}

// RunNTimes replays the same request n times against fresh isolated databases
// and returns the canonical versions hash from each run. All hashes must be
// identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, request Request, n int) ([]Result, error) {
	return app.RunNTimes(ctx, request, n)
}

// AllHashesEqual reports whether every result has the same VersionsHash.
func AllHashesEqual(results []Result) bool {
	return domain.AllHashesEqual(results)
}
