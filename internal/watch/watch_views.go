package watch

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// WatchView is one watch an approved command installed, with its allowance
// and every time it fired.
type WatchView = app.WatchView

// FireView is one evidence event that fired a watch.
type FireView = app.FireView

// Watch reads one of the tenant's watches with its fires; found is false when
// no such watch was installed. It only reads.
func Watch(ctx context.Context, db *storage.DB, tenantID, watchID string) (WatchView, bool, error) {
	return app.Watch(ctx, store.Reader(db), tenantID, watchID)
}

// InstalledWatchID returns the watch identity an install effect reported in its
// provider result, or empty when the result names no watch.
func InstalledWatchID(providerResult []byte) string {
	return app.InstalledWatchID(providerResult)
}
