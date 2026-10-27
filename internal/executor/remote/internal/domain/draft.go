package domain

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var ErrDraftUnterminated = errors.New("the draft stream ended without a proposal or a reason for none")

func DraftWireRequest(request draftport.Request) (*runtimev1.DraftRequest, error) {
	cases, err := draftWireCases(request.Cases)
	if err != nil {
		return nil, err
	}
	return &runtimev1.DraftRequest{
		ProtocolVersion: worker.ProtocolVersion, DraftId: request.DraftID, TenantId: request.TenantID,
		Key:   &runtimev1.DraftKey{DiagnosisCode: request.Key.DiagnosisCode, KindDigest: request.Key.KindDigest, Intent: request.Key.Intent, Preset: request.Key.Preset},
		Cases: cases, CurrentRules: draftWireRules(request.CurrentRules), AllowedFeatures: request.AllowedFeatures,
		Budget: &runtimev1.DraftBudget{WallTime: durationpb.New(request.Budget.WallTime), ModelCalls: int64(request.Budget.ModelCalls),
			InputTokens: request.Budget.InputTokens, OutputTokens: request.Budget.OutputTokens},
		SpecSha256: []byte(request.SpecDigest),
	}, nil
}

func draftWireCases(cases []draftport.Case) ([]*runtimev1.DraftCase, error) {
	wire := make([]*runtimev1.DraftCase, 0, len(cases))
	for _, c := range cases {
		features, err := json.Marshal(c.Features)
		if err != nil {
			return nil, fmt.Errorf("encode the features of case %s: %w", c.EpisodeID, err)
		}
		wire = append(wire, &runtimev1.DraftCase{EpisodeId: c.EpisodeID, DiagnosisCode: c.DiagnosisCode, Intent: c.Intent,
			Preset: c.Preset, Verdict: c.Verdict, FeaturesJson: features, Summary: c.Summary})
	}
	return wire, nil
}

func draftWireRules(rules []draftport.Rule) []*runtimev1.DraftRule {
	wire := make([]*runtimev1.DraftRule, 0, len(rules))
	for _, rule := range rules {
		wire = append(wire, &runtimev1.DraftRule{RuleId: rule.RuleID, Version: int64(rule.Version), When: rule.When, Mode: rule.Mode})
	}
	return wire
}

type DraftReading struct {
	Reply draftport.Reply
	Ended bool
}

func (d *DraftReading) Read(event *runtimev1.DraftEvent) error {
	if d.Ended {
		return fmt.Errorf("the draft stream sent an event after its terminal event")
	}
	d.addUsage(event.GetUsage())
	switch {
	case event.GetProposal() != nil:
		d.Reply.Proposal, d.Ended = proposalOf(event.GetProposal()), true
	case event.GetNoProposal() != nil:
		d.Reply.NoProposal, d.Ended = event.GetNoProposal().GetReason(), true
	}
	return nil
}

func (d *DraftReading) addUsage(usage *runtimev1.Usage) {
	d.Reply.Usage.InputTokens += usage.GetInputTokens()
	d.Reply.Usage.OutputTokens += usage.GetOutputTokens()
	d.Reply.Usage.CostMicrounits += usage.GetCostMicrounits()
}

func proposalOf(proposal *runtimev1.PlaybookProposal) *draftport.Proposal {
	return &draftport.Proposal{DiagnosisCode: proposal.GetDiagnosisCode(), Intent: proposal.GetIntent(), Preset: proposal.GetPreset(),
		When: proposal.GetWhen(), CitedEpisodeIDs: proposal.GetCitedEpisodeIds(), Rationale: proposal.GetRationale()}
}

func (d *DraftReading) Result() (draftport.Reply, error) {
	if !d.Ended {
		return d.Reply, ErrDraftUnterminated
	}
	return d.Reply, nil
}
