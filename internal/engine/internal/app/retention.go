package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

func PruneHistory(ctx context.Context, retention store.Retention, before time.Time) (domain.PruneReport, error) {
	report, err := retention.Prune(ctx, before)
	if err != nil {
		return report, fmt.Errorf("prune stream history before %s: %w", before, err)
	}
	return report, nil
}
