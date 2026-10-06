package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// AuthorizeDispatch is the read-only final readiness gate for one command's
// tenant and target.
func AuthorizeDispatch(ctx context.Context, s store.Store, reader store.Interlock, tenantID, target string) error {
	if !s.Configured() || reader == nil {
		return domain.ErrDispatchGateNotConfigured
	}
	return s.AssertInterlock(ctx, reader, tenantID, target)
}
