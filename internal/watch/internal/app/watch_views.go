package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// WatchView is one installed watch with its fires.
type WatchView = domain.WatchView

// FireView is one evidence event that fired a watch.
type FireView = domain.FireView

// Watch reads one of the tenant's watches with its fires.
func Watch(ctx context.Context, s store.Store, tenantID, watchID string) (domain.WatchView, bool, error) {
	return s.Watch(ctx, tenantID, watchID) //nolint:wrapcheck // The store names the failed read.
}
