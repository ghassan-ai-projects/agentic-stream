package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
	"strings"
	"time"
)

func (s *Scheduler) findTrigger(name string) (spec.Trigger, error) {
	for _, tr := range s.spec.Cognition.Triggers {
		if tr.Name == name {
			return tr, nil
		}
	}
	return spec.Trigger{}, fmt.Errorf("trigger %q not found", name)
}

func (s *Scheduler) latestAdmittedTime(ctx context.Context, tx *sql.Tx, situationID, triggerName, excludeTriggerID string) (*time.Time, error) {
	var evaluatedAt string
	if err := tx.QueryRowContext(ctx, `
		SELECT evaluated_at FROM trigger_evaluations
		WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted' AND trigger_id != ?
		ORDER BY evaluated_at DESC LIMIT 1`,
		situationID, triggerName, excludeTriggerID,
	).Scan(&evaluatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest admitted: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, evaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse evaluated_at: %w", err)
	}
	return &t, nil
}

func (s *Scheduler) countPending(ctx context.Context, tx *sql.Tx, tenantID string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'",
		tenantID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending count: %w", err)
	}
	return count, nil
}

func (s *Scheduler) countPendingSameTrigger(ctx context.Context, tx *sql.Tx, situationID, triggerName string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scheduler_items si
		JOIN trigger_evaluations te ON te.trigger_id = si.trigger_id
		WHERE si.situation_id = ? AND te.trigger_name = ? AND si.status = 'pending'`,
		situationID, triggerName,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending same-trigger count: %w", err)
	}
	return count, nil
}

func (s *Scheduler) insertItem(ctx context.Context, tx *sql.Tx, item Item, tenantID string) error {
	notBefore := sql.NullString{}
	if item.NotBefore != nil {
		notBefore = sql.NullString{String: item.NotBefore.Format(time.RFC3339Nano), Valid: true}
	}
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	args := []any{
		item.SchedulerItemID, item.TriggerID, tenantID, item.SituationID, item.SituationVersion, item.Kind,
		item.Lane, item.Priority, item.Status, s.dedupeKey(item.SituationID, item.SituationVersion, item.TriggerID),
		notBefore, item.ExpiresAt.Format(time.RFC3339Nano), now, now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
			kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id) DO UPDATE SET
			kind = excluded.kind,
			situation_version = excluded.situation_version,
			lane = excluded.lane,
			priority = excluded.priority,
			status = excluded.status,
			dedupe_key = excluded.dedupe_key,
			not_before = excluded.not_before,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`,
		args...,
	); err != nil {
		if isSchedulerItemIDConflict(err) {
			if _, retryErr := tx.ExecContext(ctx, `
				INSERT INTO scheduler_items (
					scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
					kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(scheduler_item_id) DO NOTHING`, args...); retryErr != nil {
				return fmt.Errorf("ignore scheduler item ID conflict: %w", retryErr)
			}
			return nil
		}
		return fmt.Errorf("upsert scheduler item: %w", err)
	}
	return nil
}

func isSchedulerItemIDConflict(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqlite3.SQLITE_CONSTRAINT_UNIQUE:
		return strings.Contains(err.Error(), "UNIQUE constraint failed: scheduler_items.scheduler_item_id")
	default:
		return false
	}
}

func (s *Scheduler) dedupeKey(situationID string, version int, triggerID string) []byte {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s", situationID, version, triggerID)
	return h.Sum(nil)
}

func (s *Scheduler) itemID() string {
	return s.idGen.New(ids.PrefixScheduler)
}
