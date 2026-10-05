package replay

import (
	"context"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// Run replays tracePath against specPath and returns the canonical result.
func Run(ctx context.Context, dbPath, specPath, tracePath, tenantID string) (Result, error) {
	return app.Run(ctx, dbPath, specPath, tracePath, tenantID)
}

// RunMode executes a replay mode without accepting credentials, effectors, or
// a resolver. Only counterfactual simulation may be added at a higher layer.
func RunMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string, capabilities ...Capabilities) (Result, error) {
	return app.RunMode(ctx, mode, dbPath, specPath, tracePath, tenantID, capabilities...)
}

// RunNTimes replays the same trace n times against fresh isolated databases
// and returns the canonical versions hash from each run. All hashes must be
// identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, specPath, tracePath, tenantID string, n int) ([]Result, error) {
	return app.RunNTimes(ctx, specPath, tracePath, tenantID, n)
}

// AllHashesEqual reports whether every result has the same VersionsHash.
func AllHashesEqual(results []Result) bool {
	return domain.AllHashesEqual(results)
}
