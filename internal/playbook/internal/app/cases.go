package app

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

func (s *Service) settleCases(ctx context.Context, tx *store.Tx, now time.Time) error {
	if err := s.openCases(ctx, tx, now); err != nil {
		return err
	}
	pending, err := tx.PendingCases(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, c := range pending {
		if err := s.verifyCase(ctx, tx, c, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) openCases(ctx context.Context, tx *store.Tx, now time.Time) error {
	candidates, err := tx.CaseCandidates(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		candidate.KindDigest = s.entityKindDigest(candidate.EntityID)
		c := s.rules.NewCase(candidate)
		if err := tx.RecordCase(ctx, c, now); err != nil {
			return err
		}
		if err := s.switchOffIneffective(ctx, tx, c, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) entityKindDigest(entityID string) string {
	if s.entityKinds == nil {
		return ""
	}
	return s.entityKinds(entityID)
}

func (s *Service) verifyCase(ctx context.Context, tx *store.Tx, c domain.Case, now time.Time) error {
	evidence, err := tx.CaseEvidence(ctx, c)
	if err != nil {
		return err
	}
	c.Verdict, c.Reason = c.Verify(evidence)
	if c.Verdict == domain.CasePending {
		return nil
	}
	if err := tx.SettleCase(ctx, c, now); err != nil {
		return err
	}
	return s.switchOffIneffective(ctx, tx, c, now)
}

func (s *Service) switchOffIneffective(ctx context.Context, tx *store.Tx, c domain.Case, now time.Time) error {
	if !c.RuleHandled() || c.Verdict != domain.CaseNotVerified {
		return nil
	}
	return tx.OpenCircuit(ctx, domain.NoEffectCircuit(c, now))
}
