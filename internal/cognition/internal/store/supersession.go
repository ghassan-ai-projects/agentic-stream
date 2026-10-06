package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ReplacementVersion is the Situation's Current Version, which supersedes
// older pending work, with its trace context.
type ReplacementVersion struct {
	SituationID, TenantID   string
	Version                 int
	Traceparent, Tracestate string
}

type SupersededItem struct {
	ID      string
	Version int
}

// loadSupersession reads the replacement version and the open items it
// supersedes.
func (t *Tx) LoadSupersession(ctx context.Context, situationID, triggerName string) (ReplacementVersion, []SupersededItem, error) {
	replacement, err := loadReplacement(ctx, t.tx, situationID)
	if err != nil {
		return ReplacementVersion{}, nil, err
	}
	items, err := supersededItems(ctx, t.tx, situationID, triggerName)
	if err != nil {
		return ReplacementVersion{}, nil, err
	}
	return replacement, items, nil
}

func loadReplacement(ctx context.Context, tx *sql.Tx, situationID string) (ReplacementVersion, error) {
	r := ReplacementVersion{SituationID: situationID}
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, current_version FROM situations WHERE situation_id = ?", situationID).Scan(&r.TenantID, &r.Version); err != nil {
		return ReplacementVersion{}, fmt.Errorf("load supersession situation: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT traceparent, tracestate FROM situation_versions WHERE situation_id = ? AND version = ?", situationID, r.Version).Scan(&traceparent, &tracestate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ReplacementVersion{}, fmt.Errorf("load supersession trace: %w", err)
	}
	r.Traceparent, r.Tracestate = traceparent.String, tracestate.String
	return r, nil
}

// supersededItems lists the trigger's pending and admitted scheduler items
// for the Situation.
func supersededItems(ctx context.Context, tx *sql.Tx, situationID, triggerName string) ([]SupersededItem, error) {
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

func scanSupersededItem(rows *sql.Rows) (SupersededItem, error) {
	var item SupersededItem
	if err := rows.Scan(&item.ID, &item.Version); err != nil {
		return SupersededItem{}, fmt.Errorf("scan superseded scheduler item: %w", err)
	}
	return item, nil
}

// coalesceTriggerWork coalesces the trigger's open scheduler items,
// supersedes their live episodes, and cancels those episodes' attempts.
func (t *Tx) CoalesceTriggerWork(ctx context.Context, situationID, triggerName, now string) error {
	if err := scheduleledger.Coalesce(ctx, t.tx, situationID, triggerName, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	if err := episodeledger.SupersedeCoalesced(ctx, t.tx, situationID, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (t *Tx) AnnounceSupersededItem(ctx context.Context, replacement ReplacementVersion, item SupersededItem, now time.Time) error {
	if err := notify.AppendLifecycleEvent(ctx, t.tx, supersededItemEvent(replacement, item, now)); err != nil {
		return fmt.Errorf("append situation superseded notification: %w", err)
	}
	return nil
}

func supersededItemEvent(replacement ReplacementVersion, item SupersededItem, now time.Time) notify.LifecycleEvent {
	situationID, tenantID := replacement.SituationID, replacement.TenantID
	trace := contractsv1.TraceContext{Traceparent: replacement.Traceparent, Tracestate: replacement.Tracestate}
	return notify.LifecycleEvent{
		ID:           "situation.superseded:" + situationID + ":" + fmt.Sprint(item.Version) + ":" + fmt.Sprint(replacement.Version) + ":" + item.ID,
		TenantID:     tenantID,
		Type:         notify.TypeSituationSuperseded,
		Subject:      "situation/" + situationID,
		PartitionKey: situationID,
		Data:         supersededItemData(replacement, item),
		At:           now.UTC(),
		Trace:        trace,
	}
}

func supersededItemData(replacement ReplacementVersion, item SupersededItem) map[string]any {
	situationID, tenantID := replacement.SituationID, replacement.TenantID
	return map[string]any{
		"tenant_id": tenantID, "situation_id": situationID,
		"superseded_version": item.Version, "replacement_version": replacement.Version,
		"reason": "newer_situation_version_admitted", "source_authority": notify.SourceForTenant(tenantID),
	}
}
