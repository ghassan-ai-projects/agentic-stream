package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func PruneIgnoredEvaluations(ctx context.Context, tx *store.Tx, tenantID string, before time.Time) (int64, error) {
	removed, err := tx.PruneIgnoredEvaluations(ctx, tenantID, before)
	if err != nil {
		return 0, fmt.Errorf("prune ignored evaluations before %s: %w", before, err)
	}
	return removed, nil
}
