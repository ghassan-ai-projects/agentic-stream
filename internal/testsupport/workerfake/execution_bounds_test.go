package workerfake

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestExecutionContextEndsAtTheEarliestOfParentDeadlineAndWallTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		parent, request, budget time.Duration
		bound                   time.Duration
	}{
		{"parent first", 10 * time.Second, 20 * time.Second, 30 * time.Second, 10 * time.Second},
		{"request deadline first", 30 * time.Second, 10 * time.Second, 20 * time.Second, 10 * time.Second},
		{"wall time first", 30 * time.Second, 20 * time.Second, 10 * time.Second, 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			parent, parentCancel := context.WithDeadline(t.Context(), start.Add(tt.parent))
			defer parentCancel()
			req := validRequest()
			req.Deadline = timestamppb.New(start.Add(tt.request))
			req.Budget.WallTime = durationpb.New(tt.budget)

			ctx, cancel, err := boundedExecutionContext(parent, req)
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()

			deadline, ok := ctx.Deadline()
			if !ok || deadline.Before(start.Add(tt.bound)) || deadline.After(time.Now().Add(tt.bound)) {
				t.Fatalf("deadline = %v, want about %v after the start", deadline, tt.bound)
			}
			cancel()
			if !errors.Is(ctx.Err(), context.Canceled) || parent.Err() != nil {
				t.Fatalf("after cancel: child %v, parent %v; want only the child canceled", ctx.Err(), parent.Err())
			}
		})
	}
}

func TestInvalidWallBudgetCancelsTheDeadlineItWasGiven(t *testing.T) {
	t.Parallel()
	canceled := false
	req := &runtimev1.EpisodeRequest{Budget: &runtimev1.EpisodeBudget{WallTime: durationpb.New(0)}}
	ctx, cancel, err := applyWallBudget(t.Context(), func() { canceled = true }, req)
	if ctx != nil || cancel != nil || status.Code(err) != codes.InvalidArgument || !canceled {
		t.Fatalf("applyWallBudget() = %v, %v, %v (canceled=%v); want an InvalidArgument refusal that released the deadline", ctx, cancel != nil, err, canceled)
	}
}
