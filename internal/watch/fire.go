package watch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const (
	watchExpireAttempts = 3
	watchExpireBackoff  = 250 * time.Millisecond
)

// Fire records one event-driven watch firing exactly once and decrements its
// bounded allowance. It returns false for expired, disabled, duplicate, or
// expression-evaluation-error no-fires.
func (e *Effector) Fire(ctx context.Context, watchID, eventID, situationID, target string, features map[string]any) (bool, error) {
	if e == nil || e.db == nil || watchID == "" || eventID == "" {
		return false, fmt.Errorf("watch identity is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	var fired bool
	err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		fired, err = e.fireTx(ctx, tx, watchID, eventID, situationID, target, features, now)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("watch fire transaction: %w", err)
	}
	return fired, nil
}

func (e *Effector) fireTx(ctx context.Context, tx *sql.Tx, watchID, eventID, situationID, target string, features map[string]any, now string) (bool, error) {
	if err := e.assertGuards(ctx, tx, "", ""); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
		return false, fmt.Errorf("expire due watch conditions: %w", err)
	}
	matches, err := activeWatchMatches(ctx, tx, watchID, eventID, situationID, target, features, now)
	if err != nil || !matches {
		return false, err
	}
	return recordWatchFire(ctx, tx, watchID, eventID, now)
}

// activeWatchMatches reports whether an active, unexpired watch scoped to this
// Situation and target matches the features. An expression that fails to
// evaluate is a logged no-fire, never an error.
func activeWatchMatches(ctx context.Context, tx *sql.Tx, watchID, eventID, situationID, target string, features map[string]any, now string) (bool, error) {
	var expression, storedSituationID, storedTarget string
	if err := tx.QueryRowContext(ctx, "SELECT expression, situation_id, target FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ?", watchID, now).Scan(&expression, &storedSituationID, &storedTarget); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("load active watch condition: %w", err)
	}
	if storedSituationID != situationID || storedTarget != target {
		return false, nil
	}
	return evaluateActiveWatch(ctx, expression, features, watchID, eventID, storedSituationID, storedTarget)
}

// recordWatchFire records the fire at most once per event and spends one
// unit of the watch's allowance, disabling it at zero.
func recordWatchFire(ctx context.Context, tx *sql.Tx, watchID, eventID, now string) (bool, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO watch_fires (watch_id, event_id, fired_at) SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ? AND remaining_fires > 0) ON CONFLICT(watch_id, event_id) DO NOTHING`, watchID, eventID, now, watchID, now)
	if err != nil {
		return false, fmt.Errorf("record watch fire: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count watch fire: %w", err)
	}
	if count != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE watch_conditions SET remaining_fires = remaining_fires - 1, status = CASE WHEN remaining_fires = 1 THEN 'disabled' ELSE status END, updated_at = ? WHERE watch_id = ?`, now, watchID); err != nil {
		return false, fmt.Errorf("decrement watch allowance: %w", err)
	}
	return true, nil
}

// FireEvent evaluates all active watches scoped to one event target. The
// event log remains the source of evidence; duplicate event delivery is
// absorbed by the watch_fires primary key.
func (e *Effector) FireEvent(ctx context.Context, eventID, target string, features map[string]any) (int, error) {
	if e == nil || e.db == nil || eventID == "" || target == "" {
		return 0, fmt.Errorf("watch event identity is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	candidates, err := e.eventWatches(ctx, target, now)
	if err != nil {
		return 0, err
	}
	return e.fireCandidates(ctx, eventID, target, features, candidates)
}

type watchCandidate struct{ watchID, situationID string }

func (e *Effector) eventWatches(ctx context.Context, target, now string) ([]watchCandidate, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT watch_id, situation_id FROM watch_conditions WHERE target = ? AND status = 'active' AND expires_at > ?`, target, now)
	if err != nil {
		return nil, fmt.Errorf("load watches for event: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return storage.CollectRows(rows, "watches for event", scanWatchCandidate) //nolint:wrapcheck // Preserve scan and iteration error text.
}

func scanWatchCandidate(rows *sql.Rows) (watchCandidate, error) {
	var item watchCandidate
	if err := rows.Scan(&item.watchID, &item.situationID); err != nil {
		return item, fmt.Errorf("scan watch for event: %w", err)
	}
	return item, nil
}

func (e *Effector) fireCandidates(ctx context.Context, eventID, target string, features map[string]any, candidates []watchCandidate) (int, error) {
	fired := 0
	for _, item := range candidates {
		matched, err := e.Fire(ctx, item.watchID, eventID, item.situationID, target, features)
		if err != nil {
			return fired, err
		}
		if matched {
			fired++
		}
	}
	return fired, nil
}

func evaluateActiveWatch(ctx context.Context, expression string, features map[string]any, watchID, eventID, storedSituationID, storedTarget string) (bool, error) {
	matches, err := evaluateWatchExpression(expression, features)
	if err != nil {
		slog.WarnContext(ctx, "watch expression evaluation skipped",
			"watch_id", watchID,
			"event_id", eventID,
			"situation_id", storedSituationID,
			"target", storedTarget,
			"error", err,
		)
		return false, nil
	}
	return matches, nil
}
