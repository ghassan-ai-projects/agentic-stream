package domain

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func newRequest(edit ...func(payload map[string]any)) *episodes.Request {
	req := executorconformance.FixtureRequest()
	for _, apply := range edit {
		executorconformance.EditPayload(req, apply)
	}
	return req
}

type eventScript struct {
	req      *episodes.Request
	sequence uint64
}

func scriptFor(req *episodes.Request) *eventScript { return &eventScript{req: req} }

func (s *eventScript) next(payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
	s.sequence++
	event := &runtimev1.EpisodeEvent{
		EpisodeId: s.req.EpisodeID, AttemptId: s.req.AttemptID, Fence: uint64(s.req.Fence), //nolint:gosec // The fixture fence is positive.
		Sequence: s.sequence, OccurredAt: timestamppb.New(time.Unix(10, 0)),
	}
	payload(event)
	return event
}

func (s *eventScript) started() *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Started{Started: &runtimev1.EpisodeStarted{}}
	})
}

func (s *eventScript) budget(update *runtimev1.BudgetUpdated) *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) { e.Payload = &runtimev1.EpisodeEvent_Budget{Budget: update} })
}

func (s *eventScript) decision(t *testing.T, document map[string]any) *runtimev1.EpisodeEvent {
	t.Helper()
	raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatalf("seal decision: %v", err)
	}
	return s.decisionBytes(raw, sum)
}

func (s *eventScript) decisionBytes(raw, sum []byte) *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
			DecisionJson: raw, DecisionSha256: sum, EpisodeId: e.EpisodeId, AttemptId: e.AttemptId, Fence: e.Fence,
		}}
	})
}

func (s *eventScript) terminal(status runtimev1.TerminalStatus, usage *runtimev1.Usage) *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: status, ReasonCode: "reason", Usage: usage}}
	})
}

func (s *eventScript) model() *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_ModelStarted{ModelStarted: &runtimev1.ModelStarted{}}
	})
}

func (s *eventScript) diagnostic() *runtimev1.EpisodeEvent {
	return s.next(func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Diagnostic{Diagnostic: &runtimev1.Diagnostic{Code: "note"}}
	})
}

func modelStarted() *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_ModelStarted{ModelStarted: &runtimev1.ModelStarted{}}}
}

func modelCompleted(usage *runtimev1.Usage) *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_ModelCompleted{ModelCompleted: &runtimev1.ModelCompleted{Usage: usage}}}
}

func toolExecutionStarted() *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_Tool{Tool: &runtimev1.ToolLifecycle{ExecutionStarted: true}}}
}

func toolProgress(bytesRead uint64) *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_ToolProgress{ToolProgress: &runtimev1.ToolProgress{BytesRead: bytesRead}}}
}

func budgetUpdate(update *runtimev1.BudgetUpdated) *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_Budget{Budget: update}}
}

var (
	errDeadline = context.DeadlineExceeded
	errCanceled = context.Canceled
)
