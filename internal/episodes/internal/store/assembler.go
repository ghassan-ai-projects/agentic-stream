package store

import (
	"context"
	"database/sql"
	"fmt"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
)

// SchedulerItem is one pending scheduler item as scanned.
type SchedulerItem struct {
	SchedulerItemID  string
	Kind             string
	TriggerID        string
	TenantID         string
	SituationID      string
	SituationVersion int
}

type Evaluation = domain.Evaluation

// LoadSchedulerItem reads one scheduler item inside the caller's transaction.
func LoadSchedulerItem(ctx context.Context, tx *sql.Tx, id string) (SchedulerItem, error) {
	var item SchedulerItem
	if err := tx.QueryRowContext(ctx, `
		SELECT scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version
		FROM scheduler_items WHERE scheduler_item_id = ?`,
		id,
	).Scan(&item.SchedulerItemID, &item.Kind, &item.TriggerID, &item.TenantID, &item.SituationID, &item.SituationVersion); err != nil {
		return item, fmt.Errorf("query scheduler item: %w", err)
	}
	return item, nil
}

// LoadEvaluation reads one trigger evaluation inside the caller's transaction.
func LoadEvaluation(ctx context.Context, tx *sql.Tx, triggerID string) (Evaluation, error) {
	var ev Evaluation
	if err := tx.QueryRowContext(ctx, `
		SELECT trigger_id, trigger_name, score, threshold, lane, delta_json
		FROM trigger_evaluations WHERE trigger_id = ?`,
		triggerID,
	).Scan(&ev.TriggerID, &ev.TriggerName, &ev.Score, &ev.Threshold, &ev.Lane, &ev.DeltaJSON); err != nil {
		return ev, fmt.Errorf("query evaluation: %w", err)
	}
	return ev, nil
}

// LoadSnapshot reads one situation version's snapshot document, persisted
// digest and trace context inside the caller's transaction.
func LoadSnapshot(ctx context.Context, tx *sql.Tx, situationID string, version int) (snapshotJSON, snapshotDigest []byte, traceparent, tracestate string, err error) {
	var tp, ts sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256, traceparent, tracestate FROM situation_versions
		WHERE situation_id = ? AND version = ?`,
		situationID, version).Scan(&snapshotJSON, &snapshotDigest, &tp, &ts); err != nil {
		return nil, nil, "", "", fmt.Errorf("query situation version: %w", err)
	}
	return snapshotJSON, snapshotDigest, tp.String, ts.String, nil
}

// LiveSituationVersion reads a situation's current version for the dispatch
// freshness recheck.
func LiveSituationVersion(ctx context.Context, tx *sql.Tx, tenantID, situationID string) (int64, error) {
	var liveVersion int64
	if err := tx.QueryRowContext(ctx, `
		SELECT current_version FROM situations
		WHERE tenant_id = ? AND situation_id = ?`,
		tenantID, situationID,
	).Scan(&liveVersion); err != nil {
		return 0, fmt.Errorf("recheck live situation version: %w", err)
	}
	return liveVersion, nil
}

// CountFailedAttempts counts an episode's attempts in the three failure
// statuses the retry budget counts.
func CountFailedAttempts(ctx context.Context, tx *sql.Tx, episodeID string) (int, error) {
	var failedAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, episodeID).Scan(&failedAttempts); err != nil {
		return 0, fmt.Errorf("count failed attempts: %w", err)
	}
	return failedAttempts, nil
}
