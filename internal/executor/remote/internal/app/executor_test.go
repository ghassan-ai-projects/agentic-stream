package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestExecuteConsumesAFencedStreamIntoAProducedOutcome(t *testing.T) {
	t.Parallel()
	req := workerRequest(map[string]any{"wall_time": "1m"})
	var sent *runtimev1.EpisodeRequest
	document := map[string]any{"decision_id": "d-1"}
	produce := producesDecision(document)
	worker := newCountingWorker(func(ctx context.Context, wire *runtimev1.EpisodeRequest, emit emitFunc) error {
		sent = wire
		return produce(ctx, wire, emit)
	})
	before := time.Now()

	outcome, err := worker.executor(t).Execute(t.Context(), req)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	if outcome.Status != string(episodeledger.AttemptProduced) || outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence || string(outcome.DecisionJSON) != `{"decision_id":"d-1"}` {
		t.Fatalf("Execute() outcome = %+v", outcome)
	}
	if sent.GetAttemptId() != req.AttemptID || sent.GetFence() != 1 {
		t.Fatalf("worker was sent attempt %q fence %d, want the request's fenced attempt", sent.GetAttemptId(), sent.GetFence())
	}
	deadline := sent.GetDeadline().AsTime()
	if deadline.Before(before.Add(59*time.Second)) || deadline.After(time.Now().Add(time.Minute)) {
		t.Fatalf("worker deadline = %v, want the one-minute wall time from now", deadline)
	}
}

func TestExecuteReportsAWorkerBudgetBreachAsATypedError(t *testing.T) {
	t.Parallel()
	worker := newCountingWorker(func(_ context.Context, req *runtimev1.EpisodeRequest, emit emitFunc) error {
		return emit(workerEvent(req, 2, func(e *runtimev1.EpisodeEvent) {
			e.Payload = &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{ModelCallsUsed: 2}}
		}))
	})
	outcome, err := worker.executor(t).Execute(t.Context(), workerRequest(map[string]any{"wall_time": "1m", "model_calls": 1}))
	var exceeded *episodes.BudgetExceededError
	if outcome != nil || !errors.As(err, &exceeded) || exceeded.Metric != "model_calls" {
		t.Fatalf("Execute() = %+v, %v; want a model_calls budget error", outcome, err)
	}
}

func TestExecuteRefusesAStreamWithoutATerminal(t *testing.T) {
	t.Parallel()
	worker := newCountingWorker(func(context.Context, *runtimev1.EpisodeRequest, emitFunc) error { return nil })
	_, err := worker.executor(t).Execute(t.Context(), workerRequest(nil))
	if err == nil || !strings.Contains(err.Error(), "without terminal") {
		t.Fatalf("Execute() = %v, want a missing-terminal error from the worker's stream", err)
	}
}

func TestExecuteKeepsTheWorkersStatusOnAStreamFailure(t *testing.T) {
	t.Parallel()
	worker := newCountingWorker(func(context.Context, *runtimev1.EpisodeRequest, emitFunc) error {
		return status.Error(codes.PermissionDenied, "capability refused")
	})
	outcome, err := worker.executor(t).Execute(t.Context(), workerRequest(nil))
	if outcome != nil || status.Code(err) != codes.PermissionDenied || !strings.Contains(err.Error(), "receive worker event") {
		t.Fatalf("Execute() = %+v, %v; want the PermissionDenied status wrapped with the receive step", outcome, err)
	}
}

func TestExecuteRefusesRequestsItCannotBoundBeforeContactingTheWorker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  *episodes.Request
		want string
	}{
		{"nil request", nil, "episode request is required"},
		{"no wall time", workerRequest(map[string]any{"model_calls": 3}), "invalid wall_time budget"},
		{"malformed wall time", workerRequest(map[string]any{"wall_time": "soon"}), "validate episode budget"},
		{"request that cannot be mapped to the wire", func() *episodes.Request {
			req := workerRequest(nil)
			req.Fence = 0
			return req
		}(), "build worker request: fence must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worker := newCountingWorker(declines)
			_, err := worker.executor(t).Execute(t.Context(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() = %v, want error containing %q", err, tt.want)
			}
			if worker.handshakes.Load() != 0 || worker.executions.Load() != 0 {
				t.Fatalf("worker contacted for a refused request: %d handshakes, %d executions", worker.handshakes.Load(), worker.executions.Load())
			}
		})
	}
}

func TestExecuteWithoutAWorkerClientRefusesToRun(t *testing.T) {
	t.Parallel()
	var missing *Executor
	for name, executor := range map[string]*Executor{"nil executor": missing, "nil client": New(nil, "worker-1", "runtime-1", nil)} {
		if _, err := executor.Execute(t.Context(), workerRequest(nil)); err == nil || err.Error() != "worker client is not configured" {
			t.Errorf("%s: Execute() = %v", name, err)
		}
	}
}
