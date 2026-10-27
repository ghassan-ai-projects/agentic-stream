package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	maxCitedCases  = 20
	defaultRateCap = 6
)

var ErrProposalRefused = errors.New("playbook proposal refused")

var DraftBudget = draftport.Budget{WallTime: time.Minute, ModelCalls: 6, InputTokens: 30000, OutputTokens: 5000}

func (r *Rules) DraftRequestFor(draftID, tenantID string, key Key, cases []KeyedCase) draftport.Request {
	var cited []draftport.Case
	for _, c := range slices.Backward(cases) {
		if c.Key == key && c.Verdict == CaseVerified && len(cited) < maxCitedCases {
			cited = append(cited, draftport.Case{EpisodeID: c.EpisodeID, DiagnosisCode: c.DiagnosisCode, Intent: c.Intent,
				Preset: c.Preset, Verdict: c.Verdict, Features: c.Features, Summary: c.Summary})
		}
	}
	slices.Reverse(cited)
	return draftport.Request{DraftID: draftID, TenantID: tenantID, Key: draftport.Key(key), Cases: cited,
		CurrentRules: r.draftRules(key), AllowedFeatures: r.spec.FeatureNames(), SpecDigest: r.spec.Digest, Budget: DraftBudget}
}

func (r *Rules) draftRules(key Key) []draftport.Rule {
	rules := []draftport.Rule{}
	for _, rule := range r.Rules(key) {
		rules = append(rules, draftport.Rule{RuleID: rule.ID, Version: rule.Version, When: rule.When, Mode: rule.Mode})
	}
	return rules
}

func (r *Rules) CheckProposal(request draftport.Request, proposal draftport.Proposal) error {
	if proposal.DiagnosisCode != request.Key.DiagnosisCode || proposal.Intent != request.Key.Intent || proposal.Preset != request.Key.Preset {
		return fmt.Errorf("%w: it answers another key than the draft asked for", ErrProposalRefused)
	}
	cited, err := citedCases(request, proposal.CitedEpisodeIDs)
	if err != nil {
		return err
	}
	return conditionHoldsFor(proposal.When, cited)
}

func citedCases(request draftport.Request, ids []string) ([]draftport.Case, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: it cites no case", ErrProposalRefused)
	}
	cited := make([]draftport.Case, 0, len(ids))
	for _, id := range ids {
		index := slices.IndexFunc(request.Cases, func(c draftport.Case) bool { return c.EpisodeID == id })
		if index < 0 {
			return nil, fmt.Errorf("%w: it cites %s, a case the draft did not send", ErrProposalRefused, id)
		}
		cited = append(cited, request.Cases[index])
	}
	return cited, nil
}

func conditionHoldsFor(when string, cited []draftport.Case) error {
	condition, err := compileRule(spec.Rule{ID: "proposal", When: when})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrProposalRefused, err)
	}
	for _, c := range cited {
		if !condition.matches(c.Features) {
			return fmt.Errorf("%w: its condition does not hold for the cited case %s", ErrProposalRefused, c.EpisodeID)
		}
	}
	return nil
}

type RuleIdentity struct {
	Version  int
	KindName string
}

func (r *Rules) RuleFromProposal(request draftport.Request, proposal draftport.Proposal, identity RuleIdentity) (spec.Rule, error) {
	reads, err := r.spec.ConditionReads(proposal.When)
	if err != nil {
		return spec.Rule{}, fmt.Errorf("%w: %w", ErrProposalRefused, err)
	}
	cited, err := citedCases(request, proposal.CitedEpisodeIDs)
	if err != nil {
		return spec.Rule{}, err
	}
	key := Key(request.Key)
	envelope, limits := ProposedEnvelope(featuresOf(cited), append(reads, r.context...), r.spec.Promotion.Margin())
	return spec.Rule{ID: RuleID(key), Version: identity.Version, DiagnosisCode: proposal.DiagnosisCode, When: proposal.When,
		Recommend: spec.Recommendation{Intent: proposal.Intent, Preset: proposal.Preset}, Envelope: envelope, Cap: limits,
		PinnedKind: spec.PinnedKind{Name: identity.KindName, Digest: key.KindDigest}, Mode: spec.RuleModeShadow, ShadowExit: r.spec.Promotion.DraftShadowExit(),
		RateCapPerHour: defaultRateCap, Provenance: spec.Provenance{SourceEpisodes: proposal.CitedEpisodeIDs}}, nil
}

func featuresOf(cases []draftport.Case) []map[string]any {
	features := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		features = append(features, c.Features)
	}
	return features
}

const kindIDBytes = 6

func RuleID(key Key) string {
	id := strings.ReplaceAll(key.DiagnosisCode+"."+key.Intent+"."+key.Preset, "_", "-")
	kind, err := kernel.DecodeDigest(key.KindDigest)
	if err != nil {
		return id
	}
	return id + "." + hex.EncodeToString(kind[:kindIDBytes])
}
