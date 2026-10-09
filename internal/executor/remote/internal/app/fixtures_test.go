package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type emitFunc = func(*runtimev1.EpisodeEvent) error

func workerRequest(budget map[string]any) *episodes.Request {
	req := executorconformance.FixtureRequest()
	req.EntityID = "motor-1"
	req.Traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if budget != nil {
		executorconformance.SetBudget(req, budget)
	}
	return req
}

func workerEvent(req *runtimev1.EpisodeRequest, sequence uint64, payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{
		EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		OccurredAt: timestamppb.New(time.Unix(10, 0)),
	}
	payload(event)
	return event
}

func terminalEvent(req *runtimev1.EpisodeRequest, sequence uint64, status runtimev1.TerminalStatus) *runtimev1.EpisodeEvent {
	return workerEvent(req, sequence, func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: status, ReasonCode: "complete"}}
	})
}

func declines(_ context.Context, req *runtimev1.EpisodeRequest, emit emitFunc) error {
	return emit(terminalEvent(req, 2, runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED))
}

func producesDecision(document map[string]any) workerfake.ExecuteFunc {
	return func(_ context.Context, req *runtimev1.EpisodeRequest, emit emitFunc) error {
		raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, document)
		if err != nil {
			return err
		}
		decision := workerEvent(req, 2, func(e *runtimev1.EpisodeEvent) {
			e.Payload = &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
				DecisionJson: raw, DecisionSha256: sum, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			}}
		})
		if err := emit(decision); err != nil {
			return err
		}
		return emit(terminalEvent(req, 3, runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED))
	}
}

type countingWorker struct {
	*workerfake.Server
	handshakes, executions atomic.Int32
}

func newCountingWorker(execute workerfake.ExecuteFunc, configure ...func(*workerfake.Server)) *countingWorker {
	server := &workerfake.Server{WorkerName: "worker-1", WorkerVersion: "v1"}
	counting := &countingWorker{Server: server}
	server.ExecuteFunc = func(ctx context.Context, req *runtimev1.EpisodeRequest, emit emitFunc) error {
		counting.executions.Add(1)
		return execute(ctx, req, emit)
	}
	for _, apply := range configure {
		apply(server)
	}
	return counting
}

func (c *countingWorker) Handshake(ctx context.Context, req *runtimev1.HandshakeRequest) (*runtimev1.HandshakeResponse, error) {
	c.handshakes.Add(1)
	return c.Server.Handshake(ctx, req) //nolint:wrapcheck // The fake's gRPC status is the wire contract under test.
}

func (c *countingWorker) executor(t *testing.T) *Executor {
	t.Helper()
	return New(workerfake.ConnectClient(t, c), "worker-1", "runtime-1", nil)
}
