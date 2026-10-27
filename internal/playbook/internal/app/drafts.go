package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const replayHistoryLimit = 5000

func (s *Service) requestDrafts(ctx context.Context, tx *store.Tx, now time.Time) error {
	if s.drafter == nil {
		return nil
	}
	cases, err := tx.KeyedCases(ctx, s.tenantID)
	if err != nil {
		return err
	}
	for _, key := range domain.Keys(cases) {
		if err := s.requestDraft(ctx, tx, key, cases, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) requestDraft(ctx context.Context, tx *store.Tx, key domain.Key, cases []domain.KeyedCase, now time.Time) error {
	history, err := tx.KeyHistory(ctx, s.tenantID, key)
	if err != nil || !domain.Eligible(key, cases, history, s.draftAfter, s.rules.Ruled(key)) {
		return err
	}
	return tx.RequestDraft(ctx, domain.NewDraft(s.tenantID, key, domain.VerifiedCount(key, cases)), now)
}

func (s *Service) Draft(ctx context.Context) (bool, error) {
	if s.drafter == nil {
		return false, nil
	}
	draft, request, found, err := s.startNextDraft(ctx)
	if err != nil || !found {
		return false, err
	}
	callCtx, cancel := context.WithTimeout(ctx, request.Budget.WallTime)
	reply, callErr := s.drafter.Draft(callCtx, request)
	cancel()
	return true, s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.concludeDraft(ctx, tx, draft, request, reply, callErr)
	})
}

func (s *Service) startNextDraft(ctx context.Context) (domain.Draft, draftport.Request, bool, error) {
	var draft domain.Draft
	var request draftport.Request
	var found bool
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		if draft, found, err = tx.NextDraft(ctx, s.tenantID); err != nil || !found {
			return err
		}
		request, err = s.startDraft(ctx, tx, draft)
		return err
	})
	return draft, request, found, err
}

func (s *Service) startDraft(ctx context.Context, tx *store.Tx, draft domain.Draft) (draftport.Request, error) {
	cases, err := tx.KeyedCases(ctx, s.tenantID)
	if err != nil {
		return draftport.Request{}, err
	}
	request := s.rules.DraftRequestFor(draft.DraftID, s.tenantID, draft.Key, cases)
	return request, tx.StartDraft(ctx, draft.DraftID, request, s.clock.Now().UTC())
}

func (s *Service) concludeDraft(ctx context.Context, tx *store.Tx, draft domain.Draft, request draftport.Request, reply draftport.Reply, callErr error) error {
	now := s.clock.Now().UTC()
	draft.Attempts++
	switch {
	case callErr != nil:
		return tx.FinishDraft(ctx, draft.DraftID, retryStatus(draft), callErr.Error(), reply, now)
	case reply.Proposal == nil:
		return tx.FinishDraft(ctx, draft.DraftID, domain.DraftNone, reply.NoProposal, reply, now)
	}
	return s.concludeProposal(ctx, tx, draft, request, reply, now)
}

func (s *Service) concludeProposal(ctx context.Context, tx *store.Tx, draft domain.Draft, request draftport.Request, reply draftport.Reply, now time.Time) error {
	err := s.propose(ctx, tx, draft, request, *reply.Proposal, now)
	if errors.Is(err, domain.ErrProposalRefused) {
		return tx.FinishDraft(ctx, draft.DraftID, domain.DraftRefused, err.Error(), reply, now)
	}
	if err != nil {
		return err
	}
	return tx.FinishDraft(ctx, draft.DraftID, domain.DraftProposed, "", reply, now)
}

func retryStatus(draft domain.Draft) string {
	if draft.Exhausted() {
		return domain.DraftFailed
	}
	return domain.DraftRequested
}

func (s *Service) propose(ctx context.Context, tx *store.Tx, draft domain.Draft, request draftport.Request, proposed draftport.Proposal, now time.Time) error {
	rule, err := s.proposedRule(ctx, tx, request, proposed)
	if err != nil {
		return err
	}
	report, err := s.replay(ctx, tx, rule)
	if err != nil {
		return err
	}
	intent, _ := s.rules.Spec().Intent(rule.Recommend.Intent)
	proposal, err := domain.NewProposal(draft, rule, intent.Risk, report, proposed)
	if err != nil {
		return err
	}
	return tx.RecordProposal(ctx, proposal, now)
}

func (s *Service) proposedRule(ctx context.Context, tx *store.Tx, request draftport.Request, proposed draftport.Proposal) (spec.Rule, error) {
	kinds, err := tx.KindDigests(ctx, s.tenantID)
	if err != nil {
		return spec.Rule{}, err
	}
	return s.checkedRule(request, proposed, kindNamed(kinds, request.Key.KindDigest))
}

func (s *Service) checkedRule(request draftport.Request, proposed draftport.Proposal, kindName string) (spec.Rule, error) {
	if err := s.rules.CheckProposal(request, proposed); err != nil {
		return spec.Rule{}, err
	}
	key := domain.Key(request.Key)
	identity := domain.RuleIdentity{Version: s.rules.NextVersion(domain.RuleID(key)), KindName: kindName}
	rule, err := s.rules.RuleFromProposal(request, proposed, identity)
	if err != nil {
		return spec.Rule{}, err
	}
	return s.compiledRule(rule)
}

func (s *Service) compiledRule(rule spec.Rule) (spec.Rule, error) {
	document, err := spec.MarshalPlaybook(s.rules.WithRule(rule))
	if err != nil {
		return spec.Rule{}, fmt.Errorf("write the candidate playbook: %w", err)
	}
	compiled, err := spec.CompilePlaybook(s.rules.Spec(), document)
	if err != nil {
		return spec.Rule{}, fmt.Errorf("%w: %w", domain.ErrProposalRefused, err)
	}
	return compiled.Rules[len(compiled.Rules)-1], nil
}

func (s *Service) replay(ctx context.Context, tx *store.Tx, rule spec.Rule) (domain.ReplayReport, error) {
	versions, err := tx.ReplayVersions(ctx, s.tenantID, s.rules.Spec().Situation.Type, replayHistoryLimit)
	if err != nil {
		return domain.ReplayReport{}, err
	}
	cases, err := tx.KeyedCases(ctx, s.tenantID)
	if err != nil {
		return domain.ReplayReport{}, err
	}
	return s.rules.Replay(rule, versions, cases)
}
