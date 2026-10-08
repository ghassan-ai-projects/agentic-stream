package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

// supersedePending coalesces the trigger's pending and admitted scheduler
// items for the Situation, supersedes their episodes and cancels their
// attempts, then announces each superseded older version and withdraws its
// pending approvals.
func (s *scheduler) supersedePending(ctx context.Context, tx *store.Tx, situationID, triggerName string) error {
	now := s.clk.Now()
	replacement, items, err := coalesceTriggerWork(ctx, tx, situationID, triggerName, now)
	if err != nil {
		return err
	}
	if err := s.announceSuperseded(ctx, tx, replacement, items); err != nil {
		return err
	}
	if err := tx.WithdrawSuperseded(ctx, situationID, replacement.TenantID, replacement.Version, now, s.clk); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func coalesceTriggerWork(ctx context.Context, tx *store.Tx, situationID, triggerName string, now time.Time) (store.ReplacementVersion, []store.SupersededItem, error) {
	replacement, err := tx.LoadReplacement(ctx, situationID)
	if err != nil {
		return store.ReplacementVersion{}, nil, err
	}
	items, err := tx.CoalesceTriggerWork(ctx, situationID, triggerName, now)
	return replacement, items, err
}

// announceSuperseded appends a situation.superseded notification for every
// item bound to a version older than the replacement.
func (s *scheduler) announceSuperseded(ctx context.Context, tx *store.Tx, replacement store.ReplacementVersion, items []store.SupersededItem) error {
	for _, item := range items {
		if !domain.Supersedes(replacement.Version, item.Version) {
			continue
		}
		if err := tx.AnnounceSupersededItem(ctx, replacement, item, s.clk.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}
