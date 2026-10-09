package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) LatestAdmittedTime(ctx context.Context, situationID, triggerName, excludeTriggerID string) (*time.Time, error) {
	evaluatedAt, found, err := storage.QueryOptional[string](ctx, t.tx, latestAdmittedSQL, situationID, triggerName, excludeTriggerID)
	if err != nil {
		return nil, fmt.Errorf("query latest admitted: %w", err)
	}
	if !found {
		return nil, nil
	}
	parsed, err := kernel.ParseTime(evaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse evaluated_at: %w", err)
	}
	return &parsed, nil
}

const latestAdmittedSQL = `
		SELECT e.evaluated_at FROM trigger_evaluations e
		WHERE e.situation_id = ? AND e.trigger_name = ? AND e.outcome = 'admitted' AND e.trigger_id != ?
		  AND NOT EXISTS (SELECT 1 FROM scheduler_items i WHERE i.trigger_id = e.trigger_id AND i.status IN ('pending', 'coalesced'))
		ORDER BY e.evaluated_at DESC LIMIT 1`

func (t *Tx) CountPending(ctx context.Context, tenantID string) (int, error) {
	var count int
	if err := t.tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'",
		tenantID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending count: %w", err)
	}
	return count, nil
}

func (t *Tx) CountPendingSameTrigger(ctx context.Context, situationID, triggerName string) (int, error) {
	var count int
	if err := t.tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scheduler_items si
		JOIN trigger_evaluations te ON te.trigger_id = si.trigger_id
		WHERE si.situation_id = ? AND te.trigger_name = ? AND si.status = 'pending'`,
		situationID, triggerName,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending same-trigger count: %w", err)
	}
	return count, nil
}
