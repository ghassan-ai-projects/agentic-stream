package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
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

func (s *Scheduler) dedupeKey(situationID string, version int, triggerID string) []byte {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s", situationID, version, triggerID)
	return h.Sum(nil)
}

func (s *Scheduler) itemID() string {
	return s.idGen.New(ids.PrefixScheduler)
}

func (s *Scheduler) insertItem(ctx context.Context, tx *sql.Tx, item scheduleledger.Item, tenantID string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	key := s.dedupeKey(item.SituationID, item.SituationVersion, item.TriggerID)
	if err := scheduleledger.Upsert(ctx, tx, item, tenantID, key, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
