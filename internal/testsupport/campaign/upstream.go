package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var errNoDraftAnswer = errors.New("the worker ended the draft without an answer")

type usage struct {
	InputTokens    uint64 `json:"input_tokens"`
	OutputTokens   uint64 `json:"output_tokens"`
	CostMicrounits uint64 `json:"cost_microunits,omitempty"`
}

func usageOf(wire *runtimev1.Usage) *usage {
	if wire == nil {
		return nil
	}
	return &usage{InputTokens: wire.GetInputTokens(), OutputTokens: wire.GetOutputTokens(), CostMicrounits: wire.GetCostMicrounits()}
}

func (u *usage) wire() *runtimev1.Usage {
	if u == nil {
		return nil
	}
	return &runtimev1.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CostMicrounits: u.CostMicrounits}
}

func forwardEpisode(ctx context.Context, upstream runtimev1.EpisodeWorkerClient, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) (answer, error) {
	stream, err := upstream.Execute(ctx, req)
	if err != nil {
		return answer{}, fmt.Errorf("forward the episode: %w", err)
	}
	var given answer
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return given, nil
		}
		if err != nil {
			return answer{}, fmt.Errorf("read the forwarded episode: %w", err)
		}
		if err := relay(&given, event, emit); err != nil {
			return answer{}, err
		}
	}
}

func relay(given *answer, event *runtimev1.EpisodeEvent, emit func(*runtimev1.EpisodeEvent) error) error {
	if err := given.read(event); err != nil {
		return err
	}
	if err := emit(event); err != nil {
		return fmt.Errorf("pass the forwarded event on: %w", err)
	}
	return nil
}

func (a *answer) read(event *runtimev1.EpisodeEvent) error {
	if terminal := event.GetTerminal(); terminal != nil {
		a.Usage = usageOf(terminal.GetUsage())
		if terminal.GetStatus() != runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED {
			a.Failed = terminal.GetReasonCode() + ":" + terminal.GetStatus().String()
		}
	}
	if decision := event.GetDecision(); decision != nil {
		return a.readDecision(decision.GetDecisionJson())
	}
	return nil
}

type forwardedDecision struct {
	DiagnosisCode string `json:"diagnosis_code"`
	Hypothesis    string `json:"primary_hypothesis"`
	Summary       string `json:"summary"`
	Intents       []struct {
		Type       string         `json:"type"`
		Risk       string         `json:"risk_class"`
		Parameters map[string]any `json:"parameters"`
	} `json:"intents"`
}

func (a *answer) readDecision(decisionJSON []byte) error {
	var decision forwardedDecision
	if err := json.Unmarshal(decisionJSON, &decision); err != nil {
		return fmt.Errorf("decode the forwarded decision: %w", err)
	}
	a.DiagnosisCode, a.Hypothesis, a.Summary = decision.DiagnosisCode, decision.Hypothesis, decision.Summary
	for _, intent := range decision.Intents {
		parameters := maps.Clone(intent.Parameters)
		for _, bound := range []string{"entity_id", "situation_id", "situation_version"} {
			delete(parameters, bound)
		}
		a.Intents = append(a.Intents, workerfake.ProposedIntent{Type: intent.Type, Risk: intent.Risk, Parameters: parameters})
	}
	return nil
}

type draftAnswer struct {
	When   string `json:"when,omitempty"`
	Reason string `json:"reason,omitempty"`
	Failed string `json:"failed,omitempty"`
	Usage  *usage `json:"usage,omitempty"`
}

func forwardDraft(ctx context.Context, upstream runtimev1.EpisodeWorkerClient, req *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
	stream, err := upstream.DraftPlaybookRule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("forward the draft: %w", err)
	}
	var last *runtimev1.DraftEvent
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return last, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read the forwarded draft: %w", err)
		}
		last = event
	}
}

func draftAnswerOf(event *runtimev1.DraftEvent) draftAnswer {
	given := draftAnswer{Usage: usageOf(event.GetUsage())}
	switch {
	case event.GetProposal() != nil:
		given.When = event.GetProposal().GetWhen()
	case event.GetNoProposal() != nil:
		given.Reason = event.GetNoProposal().GetReason()
	case event.GetDiagnostic() != nil:
		given.Failed = event.GetDiagnostic().GetCode() + ": " + event.GetDiagnostic().GetMessage()
	}
	return given
}

func (d draftAnswer) event(ctx context.Context, req *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
	switch {
	case d.When != "":
		return workerfake.ProposeWhen(d.When, d.Usage.wire())(ctx, req)
	case d.Reason != "":
		event, err := workerfake.DeclineDraft(d.Reason)(ctx, req)
		event.Usage = d.Usage.wire()
		return event, err
	case d.Failed != "":
		return &runtimev1.DraftEvent{Event: &runtimev1.DraftEvent_Diagnostic{Diagnostic: &runtimev1.Diagnostic{Code: "replayed_failure",
			Message: d.Failed}}, Usage: d.Usage.wire()}, nil
	}
	return nil, errNoDraftAnswer
}
