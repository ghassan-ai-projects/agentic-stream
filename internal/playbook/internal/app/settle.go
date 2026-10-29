package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

func (s *Service) Settle(ctx context.Context) error {
	return s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.settleIn(ctx, tx, s.clock.Now().UTC())
	})
}

func (s *Service) settleIn(ctx context.Context, tx *store.Tx, now time.Time) error {
	if err := s.tripKindChanges(ctx, tx, now); err != nil {
		return err
	}
	if err := s.queueAudits(ctx, tx, now); err != nil {
		return err
	}
	if err := s.settleEvaluations(ctx, tx, now); err != nil {
		return err
	}
	if err := s.settleCases(ctx, tx, now); err != nil {
		return err
	}
	return s.requestDrafts(ctx, tx, now)
}

func (s *Service) queueAudits(ctx context.Context, tx *store.Tx, now time.Time) error {
	if s.queue == nil {
		return nil
	}
	due, err := tx.DueAudits(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, fire := range due {
		if err := s.queueAudit(ctx, tx, fire, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) queueAudit(ctx context.Context, tx *store.Tx, fire domain.Fire, now time.Time) error {
	ended, err := tx.EpisodeEnded(ctx, fire.EpisodeID)
	if err != nil || !ended {
		return err
	}
	itemID, err := tx.QueueAudit(ctx, s.queue, domain.AuditRequestFor(fire))
	if err != nil {
		return err
	}
	if err := tx.RecordEvaluation(ctx, domain.NewAuditEvaluation(fire, itemID), now); err != nil {
		return err
	}
	return tx.MarkAuditQueued(ctx, fire.FireID)
}

func (s *Service) settleEvaluations(ctx context.Context, tx *store.Tx, now time.Time) error {
	pending, err := tx.PendingEvaluations(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, evaluation := range pending {
		if err := s.settleEvaluation(ctx, tx, evaluation, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) settleEvaluation(ctx context.Context, tx *store.Tx, evaluation domain.Evaluation, now time.Time) error {
	decision, err := tx.Decision(ctx, evaluation.EpisodeID)
	if err != nil || !decision.Ended {
		return err
	}
	verdict, observed := s.judge(evaluation, decision)
	if err := tx.SettleEvaluation(ctx, evaluation.EvaluationID, verdict, observed, now); err != nil {
		return err
	}
	if evaluation.Kind == domain.KindAudit && verdict == domain.VerdictDisagree {
		return tx.OpenCircuit(ctx, domain.DisagreementCircuit(evaluation, *observed, now))
	}
	return nil
}

func (s *Service) judge(evaluation domain.Evaluation, decision store.EpisodeDecision) (string, *domain.Observed) {
	if !decision.Accepted {
		return domain.VerdictInconclusive, nil
	}
	observed := s.rules.Observe(decision.Decision)
	return evaluation.Recommendation.Verdict(observed), &observed
}
