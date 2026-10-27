package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

func ResolvedAfter(ctx context.Context, occurrences store.Occurrences, situationID string, version int) (time.Time, bool, error) {
	return occurrences.ResolvedAfter(ctx, situationID, version)
}

func ReopenedAfter(ctx context.Context, occurrences store.Occurrences, tenantID, entityID, situationType string, after time.Time) (time.Time, bool, error) {
	return occurrences.ReopenedAfter(ctx, tenantID, entityID, situationType, after)
}
