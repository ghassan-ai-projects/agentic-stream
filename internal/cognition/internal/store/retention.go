package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) PruneIgnoredEvaluations(ctx context.Context, tenantID string, before time.Time) (int64, error) {
	result, err := t.tx.ExecContext(ctx, pruneIgnoredEvaluationsSQL, sql.Named("tenant", tenantID), sql.Named("before", kernel.FormatTime(before)))
	if err != nil {
		return 0, fmt.Errorf("delete ignored trigger evaluations: %w", err)
	}
	removed, err := storage.RowsAffected(result)
	if err != nil {
		return 0, fmt.Errorf("delete ignored trigger evaluations: %w", err)
	}
	return removed, nil
}

const pruneIgnoredEvaluationsSQL = `
		DELETE FROM trigger_evaluations
		WHERE tenant_id = :tenant AND outcome = 'ignored' AND evaluated_at < :before
		  AND NOT EXISTS (SELECT 1 FROM scheduler_items i WHERE i.trigger_id = trigger_evaluations.trigger_id)
		  AND NOT EXISTS (SELECT 1 FROM reconsiderations r WHERE r.trigger_id = trigger_evaluations.trigger_id)`
