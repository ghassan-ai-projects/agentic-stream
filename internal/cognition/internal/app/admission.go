package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Admit persists the trigger evaluation and creates or updates the durable
// scheduler item. It runs inside the supplied transaction.
func (s *scheduler) Admit(ctx context.Context, tx *store.Tx, eval domain.Evaluation, v situations.Version, tenantID, deploymentID string) error {
	if err := s.saveEvaluation(ctx, tx, eval, tenantID, deploymentID); err != nil {
		return fmt.Errorf("save evaluation: %w", err)
	}
	// Ignored or rejected evaluations do not create queue items.
	if !eval.Admitted() {
		return nil
	}
	item, err := s.buildItem(ctx, tx, eval)
	if err != nil {
		return fmt.Errorf("build item: %w", err)
	}
	return s.enqueueOrDefer(ctx, tx, eval, item, tenantID, deploymentID)
}

// enqueueOrDefer defers the evaluation when global capacity is exhausted;
// otherwise it supersedes stale pending items for the same Situation and
// trigger (and their episodes, so the new episode fits the one-live-episode
// constraint) and queues the item.
func (s *scheduler) enqueueOrDefer(ctx context.Context, tx *store.Tx, eval domain.Evaluation, item scheduleledger.Item, tenantID, deploymentID string) error {
	full, err := s.capacityExhausted(ctx, tx, eval, tenantID)
	if err != nil {
		return err
	}
	if full {
		return s.deferEvaluation(ctx, tx, eval, tenantID, deploymentID)
	}
	if err := s.supersedePending(ctx, tx, eval.SituationID, eval.TriggerName); err != nil {
		return fmt.Errorf("supersede pending: %w", err)
	}
	if err := s.insertItem(ctx, tx, item, tenantID); err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	return nil
}

// capacityExhausted reports a full queue, unless this version replaces a
// stale pending item for the same Situation and trigger.
func (s *scheduler) capacityExhausted(ctx context.Context, tx *store.Tx, eval domain.Evaluation, tenantID string) (bool, error) {
	pending, err := tx.CountPending(ctx, tenantID)
	if err != nil {
		return false, fmt.Errorf("count pending: %w", err)
	}
	staleSameTrigger, err := tx.CountPendingSameTrigger(ctx, eval.SituationID, eval.TriggerName)
	if err != nil {
		return false, fmt.Errorf("count stale same-trigger: %w", err)
	}
	return domain.CapacityExhausted(pending, staleSameTrigger), nil
}

func (s *scheduler) deferEvaluation(ctx context.Context, tx *store.Tx, eval domain.Evaluation, tenantID, deploymentID string) error {
	domain.Defer(&eval)
	if err := s.saveEvaluation(ctx, tx, eval, tenantID, deploymentID); err != nil {
		return fmt.Errorf("save deferred evaluation: %w", err)
	}
	return nil
}

func (s *scheduler) saveEvaluation(ctx context.Context, tx *store.Tx, eval domain.Evaluation, tenantID, deploymentID string) error {
	if err := tx.UpsertEvaluation(ctx, eval, tenantID, deploymentID, s.spec.Digest); err != nil {
		return err
	}
	return tx.AnnounceEvaluation(ctx, eval, tenantID)
}

func (s *scheduler) buildItem(ctx context.Context, tx *store.Tx, eval domain.Evaluation) (scheduleledger.Item, error) {
	item := domain.NewSchedulerItem(s.itemID(), eval)
	trigger, err := domain.FindTrigger(s.spec, eval.TriggerName)
	if err != nil {
		return item, err
	}
	if err := domain.ApplyTiming(&item, trigger, s.clk.Now().UTC()); err != nil {
		return item, err
	}
	err = s.applyTriggerCooldown(ctx, tx, &item, eval, trigger)
	return item, err
}

func (s *scheduler) applyTriggerCooldown(ctx context.Context, tx *store.Tx, item *scheduleledger.Item, eval domain.Evaluation, trigger spec.Trigger) error {
	cooldown, err := domain.ParseOptionalDuration(trigger.Cooldown, 0)
	if err != nil {
		return fmt.Errorf("parse cooldown: %w", err)
	}
	if cooldown <= 0 {
		return nil
	}
	return s.applyCooldown(ctx, tx, item, eval, cooldown)
}

// applyCooldown delays the item until cooldown after the trigger's latest
// admission, when that is later than its current not-before time.
func (s *scheduler) applyCooldown(ctx context.Context, tx *store.Tx, item *scheduleledger.Item, eval domain.Evaluation, cooldown time.Duration) error {
	latest, err := tx.LatestAdmittedTime(ctx, eval.SituationID, eval.TriggerName, eval.TriggerID)
	if err != nil || latest == nil {
		return err
	}
	domain.ApplyCooldown(item, *latest, cooldown)
	return nil
}

func (s *scheduler) itemID() string {
	return s.idGen.New(ids.PrefixScheduler)
}

func (s *scheduler) insertItem(ctx context.Context, tx *store.Tx, item scheduleledger.Item, tenantID string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	key := domain.SchedulerDedupeKey(item.SituationID, item.SituationVersion, item.TriggerID)
	if err := tx.InsertItem(ctx, item, tenantID, key, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
