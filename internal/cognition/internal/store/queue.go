package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (t *Tx) LatestAdmittedTime(ctx context.Context, situationID, triggerName, excludeTriggerID string) (*time.Time, error) {
	var evaluatedAt string
	err := t.tx.QueryRowContext(ctx, latestAdmittedSQL, situationID, triggerName, excludeTriggerID).Scan(&evaluatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query latest admitted: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, evaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse evaluated_at: %w", err)
	}
	return &parsed, nil
}

const latestAdmittedSQL = `
		SELECT evaluated_at FROM trigger_evaluations
		WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted' AND trigger_id != ?
		ORDER BY evaluated_at DESC LIMIT 1`

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
