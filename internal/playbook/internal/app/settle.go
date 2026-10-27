package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

func (s *Service) Settle(ctx context.Context) (domain.Report, error) {
	var report domain.Report
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		now := s.clock.Now().UTC()
		var err error
		if report.AuditsQueued, err = s.queueAudits(ctx, tx, now); err != nil {
			return err
		}
		report.Settled, report.Disagreements, err = s.settleEvaluations(ctx, tx, now)
		return err
	})
	return report, err
}

func (s *Service) queueAudits(ctx context.Context, tx *store.Tx, now time.Time) (int, error) {
	if s.queue == nil {
		return 0, nil
	}
	due, err := tx.DueAudits(ctx, s.tenantID)
	if err != nil {
		return 0, err
	}
	return s.queueDueAudits(ctx, tx, due, now)
}

func (s *Service) queueDueAudits(ctx context.Context, tx *store.Tx, due []domain.Fire, now time.Time) (int, error) {
	queued := 0
	for _, fire := range due {
		done, err := s.queueAudit(ctx, tx, fire, now)
		if err != nil {
			return queued, err
		}
		queued += done
	}
	return queued, nil
}

func (s *Service) queueAudit(ctx context.Context, tx *store.Tx, fire domain.Fire, now time.Time) (int, error) {
	ended, err := tx.EpisodeEnded(ctx, fire.EpisodeID)
	if err != nil || !ended {
		return 0, err
	}
	itemID, err := tx.QueueAudit(ctx, s.queue, domain.AuditRequestFor(fire))
	if err != nil {
		return 0, err
	}
	if err := tx.RecordEvaluation(ctx, domain.NewAuditEvaluation(fire, itemID), now); err != nil {
		return 0, err
	}
	return 1, tx.MarkAuditQueued(ctx, fire.FireID)
}

func (s *Service) settleEvaluations(ctx context.Context, tx *store.Tx, now time.Time) (int, int, error) {
	pending, err := tx.PendingEvaluations(ctx, s.tenantID)
	if err != nil {
		return 0, 0, err
	}
	settled, disagreements := 0, 0
	for _, evaluation := range pending {
		verdict, err := s.settleEvaluation(ctx, tx, evaluation, now)
		if err != nil {
			return settled, disagreements, err
		}
		settled, disagreements = settled+boolCount(verdict != domain.VerdictPending), disagreements+boolCount(verdict == domain.VerdictDisagree)
	}
	return settled, disagreements, nil
}

func (s *Service) settleEvaluation(ctx context.Context, tx *store.Tx, evaluation domain.Evaluation, now time.Time) (string, error) {
	decision, err := tx.Decision(ctx, evaluation.EpisodeID)
	if err != nil || !decision.Ended {
		return domain.VerdictPending, err
	}
	verdict, observed := s.judge(evaluation, decision)
	if err := tx.SettleEvaluation(ctx, evaluation.EvaluationID, verdict, observed, now); err != nil {
		return verdict, err
	}
	if evaluation.Kind == domain.KindAudit && verdict == domain.VerdictDisagree {
		return verdict, tx.OpenCircuit(ctx, domain.DisagreementCircuit(evaluation, *observed, now))
	}
	return verdict, nil
}

func (s *Service) judge(evaluation domain.Evaluation, decision store.EpisodeDecision) (string, *domain.Observed) {
	if !decision.Accepted {
		return domain.VerdictInconclusive, nil
	}
	observed := s.rules.Observe(decision.Decision)
	return evaluation.Recommendation.Verdict(observed), &observed
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
