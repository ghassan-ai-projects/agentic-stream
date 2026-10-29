package app

import (
	"context"
	"time"

	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
)

func LatestEventTime(ctx context.Context, u *store.Unit, tenantID string) (time.Time, bool, error) {
	return u.LatestEventTime(ctx, tenantID)
}
