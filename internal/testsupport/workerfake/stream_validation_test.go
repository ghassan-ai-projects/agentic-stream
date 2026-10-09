package workerfake

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestStreamValidatorAdmitsAGaplessStreamEndingInOneTerminal(t *testing.T) {
	t.Parallel()
	req := validRequest()
	validator := NewStreamValidator(req, Limits{}.Resolved())
	events := []*runtimev1.EpisodeEvent{
		StartedEvent(req, "w", "v", fixedNow), decisionOf(req, 2), terminalOf(req, 3, runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED),
	}
	for index, event := range events {
		if validator.Terminated() {
			t.Fatalf("terminated before event %d", index+1)
		}
		size, err := validator.Check(event)
		if err != nil {
			t.Fatalf("event %d rejected: %v", index+1, err)
		}
		validator.Record(event, size)
	}
	if !validator.Terminated() {
		t.Fatal("terminal not recorded")
	}
	if _, err := validator.Check(StartedEvent(req, "w", "v", fixedNow)); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("event after the terminal: %v, want FailedPrecondition", err)
	}
}

func TestStreamValidatorCodesEveryDefectOfAnEvent(t *testing.T) {
	t.Parallel()
	req := validRequest()
	tests := []struct {
		name   string
		limits Limits
		event  func() *runtimev1.EpisodeEvent
		want   codes.Code
	}{
		{"nil event", Limits{}, func() *runtimev1.EpisodeEvent { return nil }, codes.InvalidArgument},
		{"sequence zero", Limits{}, func() *runtimev1.EpisodeEvent { return eventOf(req, 0, func(*runtimev1.EpisodeEvent) {}) }, codes.FailedPrecondition},
		{"sequence gap", Limits{}, func() *runtimev1.EpisodeEvent {
			return terminalOf(req, 2, runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED)
		}, codes.FailedPrecondition},
		{"foreign episode", Limits{}, func() *runtimev1.EpisodeEvent {
			event := StartedEvent(req, "w", "v", fixedNow)
			event.EpisodeId = "other"
			return event
		}, codes.PermissionDenied},
		{"stale fence", Limits{}, func() *runtimev1.EpisodeEvent {
			event := StartedEvent(req, "w", "v", fixedNow)
			event.Fence++
			return event
		}, codes.PermissionDenied},
		{"missing timestamp", Limits{}, func() *runtimev1.EpisodeEvent {
			event := StartedEvent(req, "w", "v", fixedNow)
			event.OccurredAt = nil
			return event
		}, codes.InvalidArgument},
		{"invalid timestamp", Limits{}, func() *runtimev1.EpisodeEvent {
			event := StartedEvent(req, "w", "v", fixedNow)
			event.OccurredAt = &timestamppb.Timestamp{Seconds: 1 << 60}
			return event
		}, codes.InvalidArgument},
		{"missing payload", Limits{}, func() *runtimev1.EpisodeEvent {
			event := StartedEvent(req, "w", "v", fixedNow)
			event.Payload = nil
			return event
		}, codes.InvalidArgument},
		{"event over the size limit", Limits{MaxEventBytes: 1}, func() *runtimev1.EpisodeEvent { return StartedEvent(req, "w", "v", fixedNow) }, codes.ResourceExhausted},
		{"stream over the byte limit", Limits{MaxStreamBytes: 1}, func() *runtimev1.EpisodeEvent { return StartedEvent(req, "w", "v", fixedNow) }, codes.ResourceExhausted},
		{"decision of another attempt", Limits{}, func() *runtimev1.EpisodeEvent {
			event := decisionOf(req, 1)
			event.GetDecision().AttemptId = "other"
			return event
		}, codes.PermissionDenied},
		{"decision without a document", Limits{}, func() *runtimev1.EpisodeEvent {
			event := decisionOf(req, 1)
			event.GetDecision().DecisionJson = nil
			return event
		}, codes.PermissionDenied},
		{"decision with a short digest", Limits{}, func() *runtimev1.EpisodeEvent {
			event := decisionOf(req, 1)
			event.GetDecision().DecisionSha256 = []byte(strings.Repeat("x", 31))
			return event
		}, codes.PermissionDenied},
		{"terminal without a status", Limits{}, func() *runtimev1.EpisodeEvent {
			return terminalOf(req, 1, runtimev1.TerminalStatus_TERMINAL_STATUS_UNSPECIFIED)
		}, codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewStreamValidator(req, tt.limits.Resolved())
			if _, err := validator.Check(tt.event()); status.Code(err) != tt.want {
				t.Fatalf("Check() = %v, want code %v", err, tt.want)
			}
		})
	}
}

func TestStartedEventOpensTheStreamAsTheWorker(t *testing.T) {
	t.Parallel()
	req := validRequest()
	event := StartedEvent(req, "worker", "v1", fixedNow)
	if event.GetSequence() != 1 || event.GetEpisodeId() != req.GetEpisodeId() || event.GetAttemptId() != req.GetAttemptId() || event.GetFence() != req.GetFence() ||
		event.GetStarted().GetWorkerName() != "worker" || event.GetStarted().GetWorkerVersion() != "v1" || !event.GetOccurredAt().AsTime().Equal(fixedNow) {
		t.Fatalf("StartedEvent() = %v", event)
	}
}

func TestStreamValidatorStopsAtTheEventLimit(t *testing.T) {
	t.Parallel()
	req := validRequest()
	validator := NewStreamValidator(req, Limits{MaxEvents: 1}.Resolved())
	first := StartedEvent(req, "w", "v", fixedNow)
	size, err := validator.Check(first)
	if err != nil {
		t.Fatal(err)
	}
	validator.Record(first, size)
	if _, err := validator.Check(decisionOf(req, 2)); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("Check() after the event limit = %v, want ResourceExhausted", err)
	}
}
