package workerfake

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestServerHandshakeAdvertisesItsIdentityFeaturesAndLimits(t *testing.T) {
	t.Parallel()
	server := newServer(produces)
	server.MaxRequestBytes = 123
	client := runtimev1.NewEpisodeWorkerClient(Connect(t, server))
	request := validHandshake()
	request.RequestedFeatures = []string{"trace_context"}

	response, err := client.Handshake(t.Context(), request)
	if err != nil {
		t.Fatalf("Handshake() = %v", err)
	}
	if response.GetProtocolVersion() != worker.ProtocolVersion || response.GetContractVersion() != worker.ContractVersion || response.GetWorkerName() != "test-worker" ||
		response.GetWorkerVersion() != "test-v1" || len(response.GetSupportedFeatures()) != 1 || response.GetMaxRequestBytes() != 123 || response.GetMaxEventBytes() != defaultMaxEventBytes {
		t.Fatalf("Handshake() = %v", response)
	}

	request.RequestedFeatures = []string{"unknown.v1"}
	if _, err := client.Handshake(t.Context(), request); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a feature the worker lacks: code %v, want FailedPrecondition", status.Code(err))
	}
	request.ProtocolVersion = "2.0"
	if _, err := client.Handshake(t.Context(), request); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("another protocol version: code %v, want FailedPrecondition", status.Code(err))
	}
}

func TestServerStreamsStartedThenTheHandlersEvents(t *testing.T) {
	t.Parallel()
	events, err := runEpisode(t, newServer(produces), validRequest())
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if len(events) != 3 || events[0].GetStarted().GetWorkerName() != "test-worker" || events[0].GetStarted().GetWorkerVersion() != "test-v1" ||
		events[1].GetDecision() == nil || events[2].GetTerminal() == nil {
		t.Fatalf("events = %v, want started, decision, terminal", events)
	}
	for index, event := range events {
		if event.GetSequence() != uint64(index+1) {
			t.Errorf("event %d has sequence %d", index, event.GetSequence())
		}
	}
}

func TestServerRejectsAnInvalidRequestBeforeStarting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		edit   func(*runtimev1.EpisodeRequest)
		limits func(*Server)
		want   codes.Code
	}{
		{"trace context", func(r *runtimev1.EpisodeRequest) { r.Traceparent = "not-a-trace" }, nil, codes.InvalidArgument},
		{"unbounded budget", func(r *runtimev1.EpisodeRequest) { r.Budget = &runtimev1.EpisodeBudget{} }, nil, codes.InvalidArgument},
		{"expired deadline", func(r *runtimev1.EpisodeRequest) { r.Deadline = timestamppb.New(fixedNow.Add(-time.Second)) }, nil, codes.DeadlineExceeded},
		{"request over the size limit", func(*runtimev1.EpisodeRequest) {}, func(s *Server) { s.MaxRequestBytes = 8 }, codes.ResourceExhausted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ran := false
			server := newServer(func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
				ran = true
				return nil
			})
			if tt.limits != nil {
				tt.limits(server)
			}
			req := validRequest()
			tt.edit(req)
			events, err := runEpisode(t, server, req)
			if status.Code(err) != tt.want || len(events) != 0 || ran {
				t.Fatalf("Execute() = %d events, %v (handler ran: %v); want %v before any event", len(events), err, ran, tt.want)
			}
		})
	}
}

func TestServerEnforcesTheStreamProtocolOnTheHandler(t *testing.T) {
	t.Parallel()
	emitting := func(events ...func(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent) ExecuteFunc {
		return func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
			for _, build := range events {
				if err := emit(build(req)); err != nil {
					return err
				}
			}
			return nil
		}
	}
	diagnostic := func(sequence uint64) func(*runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
		return func(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
			return eventOf(req, sequence, func(e *runtimev1.EpisodeEvent) {
				e.Payload = &runtimev1.EpisodeEvent_Diagnostic{Diagnostic: &runtimev1.Diagnostic{Code: "note"}}
			})
		}
	}
	terminal := func(sequence uint64) func(*runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
		return func(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
			return terminalOf(req, sequence, runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED)
		}
	}
	foreignDecision := func(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
		event := decisionOf(req, 2)
		event.GetDecision().AttemptId = "other"
		return event
	}
	tests := []struct {
		name    string
		handler ExecuteFunc
		tune    func(*Server)
		want    codes.Code
	}{
		{"sequence gap", emitting(diagnostic(4)), nil, codes.FailedPrecondition},
		{"no terminal", emitting(diagnostic(2)), nil, codes.FailedPrecondition},
		{"event after terminal", emitting(terminal(2), diagnostic(3)), nil, codes.FailedPrecondition},
		{"decision of another attempt", emitting(foreignDecision), nil, codes.PermissionDenied},
		{"event over the size limit", emitting(func(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
			return eventOf(req, 2, func(e *runtimev1.EpisodeEvent) {
				e.Payload = &runtimev1.EpisodeEvent_Diagnostic{Diagnostic: &runtimev1.Diagnostic{Message: strings.Repeat("x", 2000)}}
			})
		}), func(s *Server) { s.MaxEventBytes = 1000 }, codes.ResourceExhausted},
		{"too many events", emitting(diagnostic(2), diagnostic(3)), func(s *Server) { s.MaxEvents = 2 }, codes.ResourceExhausted},
		{"handler panic", func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
			panic("boom")
		}, nil, codes.Internal},
		{"handler failure without a status", func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
			return errors.New("failure")
		}, nil, codes.Internal},
		{"handler status is kept", func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
			return status.Error(codes.FailedPrecondition, "refused")
		}, nil, codes.FailedPrecondition},
		{"handler cancellation is kept", func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
			return status.Error(codes.Canceled, "stopped")
		}, nil, codes.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newServer(tt.handler)
			if tt.tune != nil {
				tt.tune(server)
			}
			events, err := runEpisode(t, server, validRequest())
			if status.Code(err) != tt.want {
				t.Fatalf("Execute() = %v, want code %v", err, tt.want)
			}
			if len(events) == 0 || events[0].GetStarted() == nil {
				t.Fatalf("events = %v, want the server's own started event first", events)
			}
		})
	}
}

func TestServerWithoutAHandlerRefusesBeforeStarting(t *testing.T) {
	t.Parallel()
	events, err := runEpisode(t, newServer(nil), validRequest())
	if status.Code(err) != codes.Unimplemented || len(events) != 0 {
		t.Fatalf("Execute() = %d events, %v; want Unimplemented before any event", len(events), err)
	}
}

func TestServerStopsAHandlerThatOutlivesTheWallTime(t *testing.T) {
	t.Parallel()
	server := newServer(func(ctx context.Context, _ *runtimev1.EpisodeRequest, _ func(*runtimev1.EpisodeEvent) error) error {
		<-ctx.Done()
		return nil
	})
	req := validRequest()
	req.Budget.WallTime = durationpb.New(time.Millisecond)
	if _, err := runEpisode(t, server, req); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("Execute() = %v, want DeadlineExceeded", err)
	}
}
