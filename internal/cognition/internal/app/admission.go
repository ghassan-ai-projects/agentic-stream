package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (s *scheduler) Admit(ctx context.Context, tx *store.Tx, eval domain.Evaluation, v situations.Version, tenantID, deploymentID string) error {
	if err := s.saveEvaluation(ctx, tx, eval, tenantID, deploymentID); err != nil {
		return fmt.Errorf("save evaluation: %w", err)
	}

	if !eval.Admitted() {
		return nil
	}
	item, err := s.buildItem(ctx, tx, eval)
	if err != nil {
		return fmt.Errorf("build item: %w", err)
	}
	return s.enqueueOrDefer(ctx, tx, eval, item, tenantID, deploymentID)
}

func (s *scheduler) enqueueOrDefer(ctx context.Context, tx *store.Tx, eval domain.Evaluation, item episodeledger.SchedulerItem, tenantID, deploymentID string) error {
	full, err := s.capacityExhausted(ctx, tx, eval, tenantID)
	if err != nil {
		return fmt.Errorf("check scheduler capacity: %w", err)
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
		return fmt.Errorf("record trigger evaluation %s: %w", eval.TriggerID, err)
	}
	if err := tx.AnnounceEvaluation(ctx, eval, tenantID); err != nil {
		return fmt.Errorf("announce trigger evaluation %s: %w", eval.TriggerID, err)
	}
	return nil
}

func (s *scheduler) buildItem(ctx context.Context, tx *store.Tx, eval domain.Evaluation) (episodeledger.SchedulerItem, error) {
	item := domain.NewSchedulerItem(eval)
	trigger, err := domain.FindTrigger(s.spec, eval.TriggerName)
	if err != nil {
		return item, fmt.Errorf("find trigger for item %s: %w", item.SchedulerItemID, err)
	}
	latest, err := s.latestAdmission(ctx, tx, eval, trigger)
	if err != nil {
		return item, err
	}
	if err := domain.ApplyTiming(&item, trigger, s.clk.Now().UTC(), latest); err != nil {
		return item, fmt.Errorf("time scheduler item %s: %w", item.SchedulerItemID, err)
	}
	return item, nil
}

func (s *scheduler) latestAdmission(ctx context.Context, tx *store.Tx, eval domain.Evaluation, trigger spec.Trigger) (*time.Time, error) {
	if trigger.Cooldown == "" {
		return nil, nil
	}
	latest, err := tx.LatestAdmittedTime(ctx, eval.SituationID, eval.TriggerName, eval.TriggerID)
	if err != nil {
		return nil, fmt.Errorf("read latest admission of trigger %s: %w", eval.TriggerName, err)
	}
	return latest, nil
}

func (s *scheduler) insertItem(ctx context.Context, tx *store.Tx, item episodeledger.SchedulerItem, tenantID string) error {
	now := s.clk.Now()
	key := domain.SchedulerDedupeKey(item.SituationID, item.SituationVersion, item.TriggerID)
	if err := tx.InsertItem(ctx, item, tenantID, key, now); err != nil {
		return fmt.Errorf("queue scheduler item %s for trigger %s: %w", item.SchedulerItemID, item.TriggerID, err)
	}
	return nil
}
