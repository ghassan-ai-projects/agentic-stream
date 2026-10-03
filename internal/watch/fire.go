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
	fired := false
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := e.assertGuards(ctx, tx, "", ""); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
			return fmt.Errorf("expire due watch conditions: %w", err)
		}
		matches, err := activeWatchMatches(ctx, tx, watchID, eventID, situationID, target, features, now)
		if err != nil || !matches {
			return err
		}
		fired, err = recordWatchFire(ctx, tx, watchID, eventID, now)
		return err
	}); err != nil {
		return false, fmt.Errorf("watch fire transaction: %w", err)
	}
	return fired, nil
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
	rows, err := e.db.QueryContext(ctx, `SELECT watch_id, situation_id FROM watch_conditions WHERE target = ? AND status = 'active' AND expires_at > ?`, target, now)
	if err != nil {
		return 0, fmt.Errorf("load watches for event: %w", err)
	}
	type candidate struct{ watchID, situationID string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.watchID, &item.situationID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan watch for event: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate watches for event: %w", err)
	}
	_ = rows.Close()
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

// Expire marks due active watches inactive without deleting their audit rows.
func (e *Effector) Expire(ctx context.Context) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("watch storage is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	return retryWhileBusy(ctx, func() error { return e.expireDue(ctx, now) })
}

// expireDue marks every active watch whose expiry has passed as expired.
func (e *Effector) expireDue(ctx context.Context, now string) error {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if e.owner != nil && e.ownerEpoch != "" {
			if err := e.owner.Assert(ctx, tx, e.ownerEpoch); err != nil {
				return fmt.Errorf("assert watch runtime owner: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
			return fmt.Errorf("expire watch conditions: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("expire watch transaction: %w", err)
	}
	return nil
}

// retryWhileBusy retries expire up to watchExpireAttempts times, waiting
// watchExpireBackoff between attempts, while SQLite reports writer
// contention.
func retryWhileBusy(ctx context.Context, expire func() error) error {
	var err error
	for attempt := 0; attempt < watchExpireAttempts; attempt++ {
		if err = expire(); err == nil {
			return nil
		}
		if !isSQLiteBusy(err) || attempt == watchExpireAttempts-1 {
			break
		}
		timer := time.NewTimer(watchExpireBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("expire watch conditions: wait for retry: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("expire watch conditions: %w", err)
}

func isSQLiteBusy(err error) bool {
	return storage.IsSQLiteBusy(err)
}
