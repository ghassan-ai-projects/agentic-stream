package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestRemoteExecutorReturnsContextEndingsTheRunnerClassifies(t *testing.T) {
	for _, ending := range []struct {
		name    string
		wall    string
		cancel  bool
		want    error
		wantRun episodeledger.AttemptStatus
	}{
		{name: "deadline", wall: "50ms", want: context.DeadlineExceeded, wantRun: episodeledger.AttemptTimedOut},
		{name: "cancel", wall: "1m", cancel: true, want: context.Canceled, wantRun: episodeledger.AttemptCancelled},
	} {
		t.Run(ending.name, func(t *testing.T) {
			started := make(chan struct{})
			client := testWorkerClient(t, func(ctx context.Context, _ *runtimev1.EpisodeRequest, _ func(*runtimev1.EpisodeEvent) error) error {
				close(started)
				<-ctx.Done()
				return status.FromContextError(ctx.Err()).Err()
			})
			req := validWorkerRequest()
			req.RequestJSON = requestWithBudget(req, map[string]any{"wall_time": ending.wall})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if ending.cancel {
				go func() {
					<-started
					cancel()
				}()
			}
			outcome, err := New(client, "worker-1", "runtime-1", nil).Execute(ctx, req)
			if outcome != nil || !errors.Is(err, ending.want) {
				t.Fatalf("outcome=%+v err=%v, want %v", outcome, err, ending.want)
			}
			if got := req.ContextEndingOutcome(err, 0).Status; got != string(ending.wantRun) {
				t.Fatalf("recorded status = %q, want %q", got, ending.wantRun)
			}
		})
	}
}
