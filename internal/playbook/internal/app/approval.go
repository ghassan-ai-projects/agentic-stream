package app

import (
	"context"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type ProposalLookup struct {
	ID       string
	Approver string
	Relay    string
	Approved bool
}

type ProposalPresentation struct {
	ID           string          `json:"id"`
	Status       string          `json:"status"`
	Proposal     domain.Proposal `json:"proposal"`
	SigningBytes []byte          `json:"signing_bytes"`
}

type ProposalResolution struct {
	ProposalLookup
	Signature []byte
	Reason    string
}

type ProposalResult struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	WrittenDigest string `json:"written_digest,omitempty"`
}

func (s *Service) PresentProposal(ctx context.Context, tx *store.Tx, lookup ProposalLookup) (ProposalPresentation, error) {
	proposal, err := s.pendingProposal(ctx, tx, lookup.ID)
	if err != nil {
		return ProposalPresentation{}, err
	}
	if err := tx.CheckSigner(ctx, s.tenantID, lookup.Approver, lookup.Relay, proposal.RiskClass); err != nil {
		return ProposalPresentation{}, err
	}
	signing, err := proposal.SigningBytes(proposal.DefaultBinding(lookup.Approver, lookup.Relay, lookup.Approved))
	return ProposalPresentation{ID: lookup.ID, Status: proposal.Status, Proposal: proposal, SigningBytes: signing}, err
}

func (s *Service) pendingProposal(ctx context.Context, tx *store.Tx, id string) (domain.Proposal, error) {
	proposal, err := tx.Proposal(ctx, s.tenantID, id)
	if err != nil {
		return proposal, err
	}
	if proposal.Status != domain.ProposalPending {
		return proposal, domain.ErrProposalResolved
	}
	return proposal, nil
}

func (s *Service) ResolveProposal(ctx context.Context, tx *store.Tx, r ProposalResolution) (ProposalResult, error) {
	proposal, err := s.pendingProposal(ctx, tx, r.ID)
	if err != nil {
		return ProposalResult{ID: r.ID}, err
	}
	binding := proposal.DefaultBinding(r.Approver, r.Relay, r.Approved)
	signing, err := proposal.SigningBytes(binding)
	if err != nil {
		return ProposalResult{ID: r.ID}, err
	}
	digest, err := tx.VerifySignature(ctx, s.tenantID, r.Approver, r.Relay, proposal.RiskClass, signing, r.Signature)
	if err != nil {
		return ProposalResult{ID: r.ID}, err
	}
	return s.settleProposal(ctx, tx, proposal, binding, digest, r.Reason)
}

func (s *Service) settleProposal(ctx context.Context, tx *store.Tx, proposal domain.Proposal, binding domain.Binding, digest, reason string) (ProposalResult, error) {
	now := s.clock.Now().UTC()
	resolution := store.Resolution{ProposalID: proposal.ProposalID, Status: domain.ProposalRejected, Approver: binding.Approver,
		Relay: binding.Relay, ApprovalDigest: digest, Reason: reason}
	if binding.Approved {
		written, err := s.writeRule(proposal.ApprovedRule(binding, digest))
		if err != nil {
			return ProposalResult{ID: proposal.ProposalID}, err
		}
		resolution.Status, resolution.WrittenDigest = domain.ProposalWritten, written
	}
	if err := tx.ResolveProposal(ctx, resolution, now); err != nil {
		return ProposalResult{ID: proposal.ProposalID}, err
	}
	return ProposalResult{ID: proposal.ProposalID, Status: resolution.Status, WrittenDigest: resolution.WrittenDigest}, nil
}

func (s *Service) writeRule(rule spec.Rule) (string, error) {
	return s.file.update(func(book *spec.CompiledPlaybook) bool {
		book.Rules = append(book.Rules, rule)
		return true
	})
}

func (s *Service) Promote(ctx context.Context) (int, error) {
	if s.file == nil {
		return 0, nil
	}
	var due []spec.Rule
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		due, err = s.exitedShadowRules(ctx, tx)
		return err
	})
	if err != nil || len(due) == 0 {
		return 0, err
	}
	_, err = s.file.update(func(book *spec.CompiledPlaybook) bool { return activate(book, due) })
	return len(due), err
}

func (s *Service) exitedShadowRules(ctx context.Context, tx *store.Tx) ([]spec.Rule, error) {
	var due []spec.Rule
	for _, rule := range s.rules.ShadowRules() {
		agree, disagree, err := tx.ShadowTallies(ctx, domain.RuleKey{ID: rule.ID, Version: rule.Version})
		if err != nil {
			return nil, err
		}
		if domain.ExitMet(rule, agree, disagree) {
			due = append(due, rule)
		}
	}
	return due, nil
}

func activate(book *spec.CompiledPlaybook, due []spec.Rule) bool {
	changed := false
	for i, rule := range book.Rules {
		if rule.Mode == spec.RuleModeShadow && slices.ContainsFunc(due, func(d spec.Rule) bool { return d.ID == rule.ID && d.Version == rule.Version }) {
			book.Rules[i].Mode, changed = spec.RuleModeActive, true
		}
	}
	return changed
}
