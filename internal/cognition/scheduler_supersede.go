package cognition

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

// supersedePending coalesces the trigger's pending and admitted scheduler
// items for the Situation, supersedes their episodes and cancels their
// attempts, then announces each superseded older version and withdraws its
// pending approvals.
func (s *Scheduler) supersedePending(ctx context.Context, tx *sql.Tx, situationID, triggerName string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	replacement, items, err := loadSupersession(ctx, tx, situationID, triggerName)
	if err != nil {
		return err
	}
	if err := coalesceTriggerWork(ctx, tx, situationID, triggerName, now); err != nil {
		return err
	}
	if err := s.announceSuperseded(ctx, tx, replacement, items); err != nil {
		return err
	}
	if err := approvalledger.WithdrawSuperseded(ctx, tx, situationID, replacement.tenantID, replacement.version, now, s.clk); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// loadSupersession reads the replacement version and the open items it
// supersedes.
func loadSupersession(ctx context.Context, tx *sql.Tx, situationID, triggerName string) (replacementVersion, []supersededItem, error) {
	replacement, err := loadReplacement(ctx, tx, situationID)
	if err != nil {
		return replacementVersion{}, nil, err
	}
	items, err := supersededItems(ctx, tx, situationID, triggerName)
	if err != nil {
		return replacementVersion{}, nil, err
	}
	return replacement, items, nil
}

// replacementVersion is the Situation's current version, which supersedes
// older pending work, with its trace context.
type replacementVersion struct {
	situationID, tenantID   string
	version                 int
	traceparent, tracestate sql.NullString
}

func loadReplacement(ctx context.Context, tx *sql.Tx, situationID string) (replacementVersion, error) {
	r := replacementVersion{situationID: situationID}
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, current_version FROM situations WHERE situation_id = ?", situationID).Scan(&r.tenantID, &r.version); err != nil {
		return replacementVersion{}, fmt.Errorf("load supersession situation: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT traceparent, tracestate FROM situation_versions WHERE situation_id = ? AND version = ?", situationID, r.version).Scan(&r.traceparent, &r.tracestate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return replacementVersion{}, fmt.Errorf("load supersession trace: %w", err)
	}
	return r, nil
}

type supersededItem struct {
	id      string
	version int
}

// supersededItems lists the trigger's pending and admitted scheduler items
// for the Situation.
func supersededItems(ctx context.Context, tx *sql.Tx, situationID, triggerName string) ([]supersededItem, error) {
	rows, err := tx.QueryContext(ctx, selectSupersededItemsSQL, situationID, situationID, triggerName)
	if err != nil {
		return nil, fmt.Errorf("find superseded scheduler items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items, err := storage.CollectRows(rows, "superseded scheduler items", scanSupersededItem)
	if err != nil {
		return nil, err //nolint:wrapcheck // CollectRows names the failed step.
	}
	// Close before the caller's writes in the same transaction.
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close superseded scheduler items: %w", err)
	}
	return items, nil
}

const selectSupersededItemsSQL = `
		SELECT scheduler_item_id, situation_version
		FROM scheduler_items
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')
		ORDER BY scheduler_item_id`

func scanSupersededItem(rows *sql.Rows) (supersededItem, error) {
	var item supersededItem
	if err := rows.Scan(&item.id, &item.version); err != nil {
		return supersededItem{}, fmt.Errorf("scan superseded scheduler item: %w", err)
	}
	return item, nil
}

// coalesceTriggerWork coalesces the trigger's open scheduler items,
// supersedes their live episodes, and cancels those episodes' attempts.
func coalesceTriggerWork(ctx context.Context, tx *sql.Tx, situationID, triggerName, now string) error {
	if err := scheduleledger.Coalesce(ctx, tx, situationID, triggerName, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	if err := episodeledger.SupersedeCoalesced(ctx, tx, situationID, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// announceSuperseded appends a situation.superseded notification for every
// item bound to a version older than the replacement.
func (s *Scheduler) announceSuperseded(ctx context.Context, tx *sql.Tx, replacement replacementVersion, items []supersededItem) error {
	for _, item := range items {
		if item.version >= replacement.version {
			continue
		}
		if err := s.announceSupersededItem(ctx, tx, replacement, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) announceSupersededItem(ctx context.Context, tx *sql.Tx, replacement replacementVersion, item supersededItem) error {
	situationID, tenantID := replacement.situationID, replacement.tenantID
	trace := contractsv1.TraceContext{Traceparent: replacement.traceparent.String, Tracestate: replacement.tracestate.String}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx,
		"situation.superseded:"+situationID+":"+fmt.Sprint(item.version)+":"+fmt.Sprint(replacement.version)+":"+item.id,
		tenantID, notify.TypeSituationSuperseded, "situation/"+situationID, situationID,
		map[string]any{
			"tenant_id": tenantID, "situation_id": situationID,
			"superseded_version": item.version, "replacement_version": replacement.version,
			"reason": "newer_situation_version_admitted", "source_authority": notify.SourceForTenant(tenantID),
		}, s.clk.Now().UTC(), trace); err != nil {
		return fmt.Errorf("append situation superseded notification: %w", err)
	}
	return nil
}
