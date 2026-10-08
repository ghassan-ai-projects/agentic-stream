package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
)

// ErrConditionMissing reports a watch that an install just wrote but cannot be read back.
var ErrConditionMissing = errors.New("watch condition is missing")

// LoadCondition reads an installed watch. It reports false when none exists.
func (tx *Tx) LoadCondition(ctx context.Context, watchID string) (domain.Condition, bool, error) {
	var stored domain.Condition
	var expiresAt string
	var remainingFires int
	err := tx.tx.QueryRowContext(ctx, `SELECT tenant_id, situation_id, situation_version, expression, target, expires_at, remaining_fires, max_fires FROM watch_conditions WHERE watch_id = ?`, watchID).Scan(&stored.TenantID, &stored.SituationID, &stored.SituationVersion, &stored.Expression, &stored.Target, &expiresAt, &remainingFires, &stored.MaxFires)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Condition{}, false, nil
	}
	if err != nil {
		return domain.Condition{}, false, fmt.Errorf("load watch condition: %w", err)
	}
	return withStoredExpiry(stored, watchID, expiresAt)
}

func withStoredExpiry(condition domain.Condition, watchID, expiresAt string) (domain.Condition, bool, error) {
	parsed, err := kernel.ParseTime(expiresAt)
	if err != nil {
		return domain.Condition{}, false, fmt.Errorf("load watch condition %s expiry: %w", watchID, err)
	}
	condition.ExpiresAt = parsed
	return condition, true, nil
}

// InsertCondition installs an active watch with its full allowance. An existing
// watch of the same ID is left untouched.
func (tx *Tx) InsertCondition(ctx context.Context, watchID string, want domain.Condition, now time.Time) error {
	at := kernel.FormatTime(now)
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO watch_conditions (
			watch_id, tenant_id, situation_id, situation_version, expression, target,
			expires_at, remaining_fires, max_fires, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(watch_id) DO NOTHING`,
		watchID, want.TenantID, want.SituationID, want.SituationVersion, want.Expression, want.Target,
		kernel.FormatTime(want.ExpiresAt), want.MaxFires, want.MaxFires, at, at); err != nil {
		return fmt.Errorf("install watch condition: %w", err)
	}
	return nil
}

// ExpireDue marks every active watch whose expiry has passed as expired,
// keeping its audit rows.
func (tx *Tx) ExpireDue(ctx context.Context, now time.Time) error {
	at := kernel.FormatTime(now)
	if _, err := tx.tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", at, at); err != nil {
		return fmt.Errorf("expire watch conditions: %w", err)
	}
	return nil
}

// LoadActive reads an active, unexpired watch. It reports false when the watch
// is absent, disabled or expired.
func (tx *Tx) LoadActive(ctx context.Context, watchID string, now time.Time) (domain.ActiveWatch, bool, error) {
	var active domain.ActiveWatch
	err := tx.tx.QueryRowContext(ctx, "SELECT expression, situation_id, target FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ?", watchID, kernel.FormatTime(now)).Scan(&active.Expression, &active.SituationID, &active.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActiveWatch{}, false, nil
	}
	if err != nil {
		return domain.ActiveWatch{}, false, fmt.Errorf("load active watch condition: %w", err)
	}
	return active, true, nil
}

// RecordFire records the fire at most once per event, and only while the watch
// is active with allowance left. It reports whether a new fire was recorded.
func (tx *Tx) RecordFire(ctx context.Context, watchID, eventID string, now time.Time) (bool, error) {
	at := kernel.FormatTime(now)
	result, err := tx.tx.ExecContext(ctx, `INSERT INTO watch_fires (watch_id, event_id, fired_at) SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ? AND remaining_fires > 0) ON CONFLICT(watch_id, event_id) DO NOTHING`, watchID, eventID, at, watchID, at)
	if err != nil {
		return false, fmt.Errorf("record watch fire: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count watch fire: %w", err)
	}
	return count == 1, nil
}

// SpendAllowance spends one unit of the watch's allowance, disabling it at zero.
func (tx *Tx) SpendAllowance(ctx context.Context, watchID string, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE watch_conditions SET remaining_fires = remaining_fires - 1, status = CASE WHEN remaining_fires = 1 THEN 'disabled' ELSE status END, updated_at = ? WHERE watch_id = ?`, kernel.FormatTime(now), watchID); err != nil {
		return fmt.Errorf("decrement watch allowance: %w", err)
	}
	return nil
}

// ActiveForTarget lists the active, unexpired watches scoped to one event
// target. It reads outside any transaction.
func (s Store) ActiveForTarget(ctx context.Context, target string, now time.Time) ([]domain.Candidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT watch_id, situation_id FROM watch_conditions WHERE target = ? AND status = 'active' AND expires_at > ?`, target, kernel.FormatTime(now))
	if err != nil {
		return nil, fmt.Errorf("load watches for event: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return storage.CollectRows(rows, "watches for event", scanCandidate) //nolint:wrapcheck // Preserve scan and iteration error text.
}

func scanCandidate(rows *sql.Rows) (domain.Candidate, error) {
	var item domain.Candidate
	if err := rows.Scan(&item.WatchID, &item.SituationID); err != nil {
		return item, fmt.Errorf("scan watch for event: %w", err)
	}
	return item, nil
}
