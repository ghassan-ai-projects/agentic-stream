package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

const rateCapWindow = time.Hour

func (s *Service) Route(ctx context.Context, tx *store.Tx, admission domain.Admission) (domain.Route, error) {
	audit, found, err := tx.PendingAudit(ctx, admission.SchedulerItemID)
	if err != nil {
		return domain.Route{}, err
	}
	if found {
		return s.routeAudit(ctx, tx, audit, admission)
	}
	return s.routeByRules(ctx, tx, admission)
}

func (s *Service) routeAudit(ctx context.Context, tx *store.Tx, audit domain.Evaluation, admission domain.Admission) (domain.Route, error) {
	if err := tx.BindEvaluationEpisode(ctx, audit.EvaluationID, admission.EpisodeID); err != nil {
		return domain.Route{}, err
	}
	return domain.Route{Kind: domain.RouteAudit, Recommendation: &audit.Recommendation}, nil
}

func (s *Service) routeByRules(ctx context.Context, tx *store.Tx, admission domain.Admission) (domain.Route, error) {
	now := s.clock.Now().UTC()
	state, err := s.ruleState(ctx, tx, admission.EventTime, now)
	if err != nil {
		return domain.Route{}, err
	}
	routing := s.rules.Route(s.rules.Features(admission.Facts, admission.Evidence), state)
	if err := s.recordMisses(ctx, tx, admission, routing, state, now); err != nil {
		return domain.Route{}, err
	}
	return s.fire(ctx, tx, admission, routing.Fire, now)
}

func (s *Service) recordMisses(ctx context.Context, tx *store.Tx, admission domain.Admission, routing domain.Routing, state domain.State, now time.Time) error {
	if err := s.recordShadows(ctx, tx, admission, routing.Shadows, now); err != nil {
		return err
	}
	if err := s.recordOutsideEnvelope(ctx, tx, admission, routing.OutsideEnvelope, now); err != nil {
		return err
	}
	return s.tripRateCap(ctx, tx, routing.RateCapped, state, now)
}

func (s *Service) ruleState(ctx context.Context, tx *store.Tx, eventTime, now time.Time) (domain.State, error) {
	if err := s.tripKindChanges(ctx, tx, now); err != nil {
		return domain.State{}, err
	}
	open, err := tx.OpenCircuits(ctx, s.tenantID)
	if err != nil {
		return domain.State{}, err
	}
	fires, err := tx.FiresSince(ctx, s.tenantID, eventTime.Add(-rateCapWindow))
	if err != nil {
		return domain.State{}, err
	}
	return domain.State{Open: open, FiresLastHour: fires}, nil
}

func (s *Service) recordShadows(ctx context.Context, tx *store.Tx, admission domain.Admission, shadows []domain.Match, now time.Time) error {
	for _, match := range shadows {
		evaluation := domain.NewShadowEvaluation(s.tenantID, admission, match, s.rules.Recommend(match))
		if err := tx.RecordEvaluation(ctx, evaluation, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) tripRateCap(ctx context.Context, tx *store.Tx, capped *domain.Match, state domain.State, now time.Time) error {
	if capped == nil {
		return nil
	}
	fires := state.FiresLastHour[domain.RuleKey{ID: capped.Rule.ID, Version: capped.Rule.Version}]
	return tx.OpenCircuit(ctx, domain.RateCapCircuit(s.tenantID, *capped, fires, now))
}

func (s *Service) fire(ctx context.Context, tx *store.Tx, admission domain.Admission, match *domain.Match, now time.Time) (domain.Route, error) {
	if match == nil {
		return domain.Route{Kind: domain.RouteModel}, nil
	}
	recommendation := s.rules.Recommend(*match)
	fire := domain.NewFire(s.tenantID, admission.EpisodeID, admission.SituationID, admission.SituationVersion,
		recommendation, match.Rule.AuditEvery, admission.EventTime)
	if err := tx.RecordFire(ctx, fire); err != nil {
		return domain.Route{}, err
	}
	return domain.Route{Kind: domain.RouteRule, Recommendation: &recommendation}, nil
}

func (s *Service) tripKindChanges(ctx context.Context, tx *store.Tx, now time.Time) error {
	if s.kinds == nil {
		return nil
	}
	for _, circuit := range s.rules.KindChanges(s.tenantID, s.kinds(), now) {
		if err := tx.OpenCircuit(ctx, circuit); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordOutsideEnvelope(ctx context.Context, tx *store.Tx, admission domain.Admission, match *domain.Match, now time.Time) error {
	if match == nil {
		return nil
	}
	if err := s.recordShadows(ctx, tx, admission, []domain.Match{*match}, now); err != nil {
		return err
	}
	rule := domain.RuleKey{ID: match.Rule.ID, Version: match.Rule.Version}
	counts, err := tx.CapCounts(ctx, rule, admission.EventTime.Add(-domain.OutOfCapWindow))
	if err != nil || !counts.MostlyOutside() {
		return err
	}
	return tx.OpenCircuit(ctx, domain.OutOfCapCircuit(s.tenantID, rule, counts, now))
}
