package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestExecuteReturnsContextEndingsTheRunnerClassifies(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			started := make(chan struct{})
			fake := newCountingWorker(func(ctx context.Context, _ *runtimev1.EpisodeRequest, _ emitFunc) error {
				close(started)
				<-ctx.Done()
				return status.FromContextError(ctx.Err()).Err()
			})
			req := workerRequest(map[string]any{"wall_time": ending.wall})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if ending.cancel {
				go func() {
					<-started
					cancel()
				}()
			}
			outcome, err := fake.executor(t).Execute(ctx, req)
			if outcome != nil || !errors.Is(err, ending.want) {
				t.Fatalf("outcome=%+v err=%v, want %v", outcome, err, ending.want)
			}
			if got := req.ContextEndingOutcome(err, 0).Status; got != string(ending.wantRun) {
				t.Fatalf("recorded status = %q, want %q", got, ending.wantRun)
			}
		})
	}
}
