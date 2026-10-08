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

// LoadReplacement reads the Situation's Current Version with its trace context.
func (t *Tx) LoadReplacement(ctx context.Context, situationID string) (ReplacementVersion, error) {
	return loadReplacement(ctx, t.tx, situationID)
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

// CoalesceTriggerWork coalesces the trigger's open scheduler items,
// supersedes their live episodes, cancels those episodes' attempts and returns
// the coalesced items.
func (t *Tx) CoalesceTriggerWork(ctx context.Context, situationID, triggerName string, now time.Time) ([]SupersededItem, error) {
	coalesced, err := episodeledger.CoalesceSchedulerItems(ctx, t.tx, situationID, triggerName, now)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if err := episodeledger.SupersedeCoalesced(ctx, t.tx, situationID, now); err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	return supersededItems(coalesced), nil
}

func supersededItems(coalesced []episodeledger.CoalescedItem) []SupersededItem {
	items := make([]SupersededItem, len(coalesced))
	for i, item := range coalesced {
		items[i] = SupersededItem{ID: item.SchedulerItemID, Version: item.SituationVersion}
	}
	return items
}

func (t *Tx) AnnounceSupersededItem(ctx context.Context, replacement ReplacementVersion, item SupersededItem, now time.Time) error {
	if err := notify.AppendLifecycleEvent(ctx, t.tx, supersededItemEvent(replacement, item, now)); err != nil {
		return fmt.Errorf("append situation superseded notification: %w", err)
	}
	return nil
}

func supersededItemEvent(replacement ReplacementVersion, item SupersededItem, now time.Time) notify.LifecycleEvent {
	trace := contractsv1.TraceContext{Traceparent: replacement.Traceparent, Tracestate: replacement.Tracestate}
	return notify.SituationSupersededEvent(replacement.TenantID, item.ID, supersededItem(replacement, item), now.UTC(), trace)
}

func supersededItem(replacement ReplacementVersion, item SupersededItem) notify.SituationSuperseded {
	return notify.SituationSuperseded{
		SituationID: replacement.SituationID, SupersededVersion: item.Version,
		ReplacementVersion: replacement.Version, Reason: "newer_situation_version_admitted",
	}
}
