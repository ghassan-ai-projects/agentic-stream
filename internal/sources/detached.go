package sources

import (
	"context"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/sources/internal/domain"
)

// PersistGrace bounds work that must finish after its caller was canceled.
const PersistGrace = domain.PersistGrace

// DetachedContext returns a context that keeps ctx's values but not its
// cancellation or deadline, bounded by PersistGrace. The caller must call the
// returned cancel function.
func DetachedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return domain.DetachedContext(ctx)
}
