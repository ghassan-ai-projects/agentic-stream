package domain

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func decisionDocument(req *episodes.Request) map[string]any {
	return map[string]any{"decision_id": "dec-1", "episode_id": req.EpisodeID, "attempt_id": req.AttemptID}
}

func acceptAll(t *testing.T, stream *Stream, events []*runtimev1.EpisodeEvent) error {
	t.Helper()
	for index, event := range events {
		err := stream.Accept(event)
		if err != nil && index < len(events)-1 {
			t.Fatalf("event %d of %d rejected early: %v", index+1, len(events), err)
		}
		if index == len(events)-1 {
			return err
		}
	}
	return nil
}

func TestStreamAcceptRejectsEventsThatBreakTheEnvelope(t *testing.T) {
	t.Parallel()
	foreign := func(change func(*runtimev1.EpisodeEvent)) func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
		return func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			event := s.started()
			change(event)
			return []*runtimev1.EpisodeEvent{event}
		}
	}
	tests := []struct {
		name          string
		maxEventBytes uint64
		events        func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent
		want          string
	}{
		{"nil event", 0, func(*testing.T, *eventScript) []*runtimev1.EpisodeEvent { return []*runtimev1.EpisodeEvent{nil} }, "nil event"},
		{"foreign episode", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.EpisodeId = "other" }), "identity or sequence mismatch"},
		{"foreign attempt", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.AttemptId = "other" }), "identity or sequence mismatch"},
		{"stale fence", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.Fence++ }), "identity or sequence mismatch"},
		{"sequence gap", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.Sequence = 2 }), "identity or sequence mismatch"},
		{"missing timestamp", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.OccurredAt = nil }), "invalid occurred_at"},
		{"invalid timestamp", 0, foreign(func(e *runtimev1.EpisodeEvent) { e.OccurredAt = &timestamppb.Timestamp{Seconds: 1 << 60} }), "invalid occurred_at"},
		{"event over the negotiated size", 1, foreign(func(*runtimev1.EpisodeEvent) {}), "exceeds negotiated size limit"},
		{"stream not started", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.diagnostic()}
		}, "did not start with episode.started"},
		{"started twice", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.started()}
		}, "duplicate started event"},
		{"event after terminal", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.terminal(runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED, nil), s.diagnostic()}
		}, "after terminal"},
		{"second terminal", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			declined := runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED
			return []*runtimev1.EpisodeEvent{s.started(), s.terminal(declined, nil), s.terminal(declined, nil)}
		}, "after terminal"},
		{"second decision", 0, func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			t.Helper()
			document := decisionDocument(s.req)
			return []*runtimev1.EpisodeEvent{s.started(), s.decision(t, document), s.decision(t, document)}
		}, "duplicate decision"},
		{"decision of another attempt", 0, func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			t.Helper()
			started := s.started()
			event := s.decision(t, decisionDocument(s.req))
			event.GetDecision().AttemptId = "other"
			return []*runtimev1.EpisodeEvent{started, event}
		}, "decision identity mismatch"},
		{"decision with a stale fence", 0, func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			t.Helper()
			started := s.started()
			event := s.decision(t, decisionDocument(s.req))
			event.GetDecision().Fence++
			return []*runtimev1.EpisodeEvent{started, event}
		}, "decision identity mismatch"},
		{"decision that is not json", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.decisionBytes([]byte("{"), make([]byte, 32))}
		}, "not valid JSON"},
		{"decision with a forged digest", 0, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			raw, _, _ := canonicaljson.Seal(canonicaljson.DomainDecision, decisionDocument(s.req))
			return []*runtimev1.EpisodeEvent{s.started(), s.decisionBytes(raw, make([]byte, 32))}
		}, "digest mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			stream := NewStream(req, &runtimev1.EpisodeBudget{}, tt.maxEventBytes)
			err := acceptAll(t, stream, tt.events(t, scriptFor(req)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Accept() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestStreamAdmissionPreservesErrorPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		limit    uint64
		mismatch bool
		want     string
	}{
		{name: "size before identity", limit: 1, mismatch: true, want: "worker event exceeds negotiated size limit"},
		{name: "identity before budget", mismatch: true, want: "worker event identity or sequence mismatch"},
		{name: "budget before payload", want: "model_calls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			script := scriptFor(req)
			script.started()
			stream := NewStream(req, &runtimev1.EpisodeBudget{MaxModelCalls: 1}, tt.limit)
			stream.sawStarted, stream.nextSequence, stream.usage.modelCalls = true, 2, 1
			event := script.model()
			if tt.mismatch {
				event.AttemptId = "foreign-attempt"
			}
			err := stream.Accept(event)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Accept() = %v, want error containing %q", err, tt.want)
			}
			if stream.nextSequence != 2 {
				t.Fatalf("rejected event advanced the sequence to %d", stream.nextSequence)
			}
		})
	}
}

func TestStreamBoundsEventCountAndBytes(t *testing.T) {
	t.Parallel()
	bigDiagnostic := func(s *eventScript, size int) *runtimev1.EpisodeEvent {
		event := s.diagnostic()
		event.GetDiagnostic().Message = strings.Repeat("x", size)
		return event
	}
	tests := []struct {
		name   string
		events func(s *eventScript) []*runtimev1.EpisodeEvent
	}{
		{"event count", func(s *eventScript) []*runtimev1.EpisodeEvent {
			events := []*runtimev1.EpisodeEvent{s.started()}
			for range worker.DefaultMaxEvents {
				events = append(events, s.diagnostic())
			}
			return events
		}},
		{"one event over the stream bytes", func(s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), bigDiagnostic(s, worker.DefaultMaxStreamBytes)}
		}},
		{"events that add up over the stream bytes", func(s *eventScript) []*runtimev1.EpisodeEvent {
			half := worker.DefaultMaxStreamBytes/2 + 1
			return []*runtimev1.EpisodeEvent{s.started(), bigDiagnostic(s, half), bigDiagnostic(s, half)}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			err := acceptAll(t, NewStream(req, &runtimev1.EpisodeBudget{}, 0), tt.events(scriptFor(req)))
			if err == nil || !strings.Contains(err.Error(), "worker stream exceeds size limit") {
				t.Fatalf("Accept() = %v, want the stream size limit", err)
			}
		})
	}
}

func TestStreamOutcomeBindsTheTerminalStatusToTheAttempt(t *testing.T) {
	t.Parallel()
	req := newRequest()
	tests := []struct {
		status runtimev1.TerminalStatus
		want   string
	}{
		{runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED, "declined"},
		{runtimev1.TerminalStatus_TERMINAL_STATUS_TIMED_OUT, req.ContextEndingOutcome(errDeadline, 0).Status},
		{runtimev1.TerminalStatus_TERMINAL_STATUS_CANCELLED, req.ContextEndingOutcome(errCanceled, 0).Status}, //nolint:misspell // Wire enum is frozen by the protocol.
		{runtimev1.TerminalStatus_TERMINAL_STATUS_FAILED, "failed"},
		{runtimev1.TerminalStatus_TERMINAL_STATUS_BUDGET_EXHAUSTED, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			t.Parallel()
			script := scriptFor(req)
			stream := NewStream(req, &runtimev1.EpisodeBudget{}, 0)
			if err := acceptAll(t, stream, []*runtimev1.EpisodeEvent{script.started(), script.terminal(tt.status, nil)}); err != nil {
				t.Fatal(err)
			}
			outcome, err := stream.Outcome()
			if err != nil {
				t.Fatalf("Outcome() = %v", err)
			}
			if outcome.Status != tt.want || outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "reason" || len(outcome.DecisionJSON) != 0 {
				t.Fatalf("Outcome() = %+v, want status %s bound to the attempt with the worker's reason and no decision", outcome, tt.want)
			}
		})
	}
}

func TestStreamOutcomeCarriesTheDecisionOnlyWithAProducedTerminal(t *testing.T) {
	t.Parallel()
	req := newRequest()
	script := scriptFor(req)
	stream := NewStream(req, &runtimev1.EpisodeBudget{}, 0)
	document := decisionDocument(req)
	raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatal(err)
	}
	events := []*runtimev1.EpisodeEvent{script.started(), script.decision(t, document), script.terminal(runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, nil)}
	if err := acceptAll(t, stream, events); err != nil {
		t.Fatal(err)
	}
	outcome, err := stream.Outcome()
	if err != nil {
		t.Fatalf("Outcome() = %v", err)
	}
	if outcome.Status != "produced" || string(outcome.DecisionJSON) != string(raw) || outcome.DecisionSHA256 != canonicaljson.EncodeDigest(sum) {
		t.Fatalf("Outcome() = %+v, want the produced decision %s", outcome, raw)
	}
}

func TestStreamOutcomeRefusesStreamsThatDoNotEndInACompleteTerminal(t *testing.T) {
	t.Parallel()
	declined := runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED
	tests := []struct {
		name   string
		budget *runtimev1.EpisodeBudget
		events func(t *testing.T, s *eventScript) []*runtimev1.EpisodeEvent
		want   string
	}{
		{"empty stream", &runtimev1.EpisodeBudget{}, func(*testing.T, *eventScript) []*runtimev1.EpisodeEvent { return nil }, "ended without terminal"},
		{"no terminal", &runtimev1.EpisodeBudget{}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started()}
		}, "ended without terminal"},
		{"produced without a decision", &runtimev1.EpisodeBudget{}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.terminal(runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, nil)}
		}, "produced worker terminal has no decision"},
		{"unspecified terminal status", &runtimev1.EpisodeBudget{}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.terminal(runtimev1.TerminalStatus_TERMINAL_STATUS_UNSPECIFIED, nil)}
		}, "unspecified terminal status"},
		{"numeric budget without budget telemetry", &runtimev1.EpisodeBudget{MaxModelCalls: 1}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.terminal(declined, nil)}
		}, "budget telemetry is missing"},
		{"token budget without usage", &runtimev1.EpisodeBudget{MaxInputTokens: 10}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.budget(&runtimev1.BudgetUpdated{}), s.terminal(declined, nil)}
		}, "budget telemetry is missing"},
		{"cost budget without usage", &runtimev1.EpisodeBudget{MaxCostMicrounits: 10}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.budget(&runtimev1.BudgetUpdated{}), s.terminal(declined, nil)}
		}, "worker cost telemetry is missing"},
		{"terminal usage regresses below the reported total", &runtimev1.EpisodeBudget{MaxCostMicrounits: 100}, func(_ *testing.T, s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{
				s.started(), s.budget(&runtimev1.BudgetUpdated{CumulativeUsage: &runtimev1.Usage{CostMicrounits: 5}}),
				s.terminal(declined, &runtimev1.Usage{CostMicrounits: 4}),
			}
		}, "cumulative usage regressed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			stream := NewStream(req, tt.budget, 0)
			events := tt.events(t, scriptFor(req))
			if len(events) > 0 {
				if err := acceptAll(t, stream, events); err != nil {
					t.Fatal(err)
				}
			}
			outcome, err := stream.Outcome()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Outcome() = %+v, %v; want error containing %q", outcome, err, tt.want)
			}
		})
	}
}

func TestStreamOutcomeReportsUsageTheWorkerCannotUnderstate(t *testing.T) {
	t.Parallel()
	declined := runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED
	tests := []struct {
		name     string
		budget   *runtimev1.EpisodeBudget
		events   func(s *eventScript) []*runtimev1.EpisodeEvent
		wantCost uint64
	}{
		{"cumulative budget report settles the cost", &runtimev1.EpisodeBudget{MaxCostMicrounits: 100}, func(s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.budget(&runtimev1.BudgetUpdated{CumulativeUsage: &runtimev1.Usage{CostMicrounits: 42}}), s.terminal(declined, nil)}
		}, 42},
		{"terminal usage settles the cost", &runtimev1.EpisodeBudget{MaxCostMicrounits: 100}, func(s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.budget(&runtimev1.BudgetUpdated{}), s.terminal(declined, &runtimev1.Usage{CostMicrounits: 7})}
		}, 7},
		{"zero priced usage is a valid report", &runtimev1.EpisodeBudget{MaxCostMicrounits: 100}, func(s *eventScript) []*runtimev1.EpisodeEvent {
			return []*runtimev1.EpisodeEvent{s.started(), s.budget(&runtimev1.BudgetUpdated{}), s.terminal(declined, &runtimev1.Usage{InputTokens: 1})}
		}, 0},
		{"per event usage counts when the report is lower", &runtimev1.EpisodeBudget{}, func(s *eventScript) []*runtimev1.EpisodeEvent {
			started := s.started()
			completed := s.next(func(e *runtimev1.EpisodeEvent) {
				e.Payload = &runtimev1.EpisodeEvent_ModelCompleted{ModelCompleted: &runtimev1.ModelCompleted{Usage: &runtimev1.Usage{CostMicrounits: 30}}}
			})
			return []*runtimev1.EpisodeEvent{started, completed, s.budget(&runtimev1.BudgetUpdated{CumulativeUsage: &runtimev1.Usage{CostMicrounits: 10}}), s.terminal(declined, nil)}
		}, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			stream := NewStream(req, tt.budget, 0)
			if err := acceptAll(t, stream, tt.events(scriptFor(req))); err != nil {
				t.Fatal(err)
			}
			outcome, err := stream.Outcome()
			if err != nil || outcome.CostMicrounits != tt.wantCost {
				t.Fatalf("Outcome() = %+v, %v; want cost %d", outcome, err, tt.wantCost)
			}
		})
	}
}

func TestTerminalUsageOverTheLimitFailsBeforeAMissingDecision(t *testing.T) {
	t.Parallel()
	req := newRequest()
	script := scriptFor(req)
	stream := NewStream(req, &runtimev1.EpisodeBudget{MaxInputTokens: 2}, 0)
	events := []*runtimev1.EpisodeEvent{script.started(), script.budget(&runtimev1.BudgetUpdated{}), script.terminal(runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, &runtimev1.Usage{InputTokens: 3})}
	if err := acceptAll(t, stream, events); err != nil {
		t.Fatal(err)
	}
	_, err := stream.Outcome()
	var exceeded *episodes.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Metric != "input_tokens" {
		t.Fatalf("Outcome() = %v, want the input_tokens budget error before the missing decision", err)
	}
}
