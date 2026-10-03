package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExecutionBoundsKeepEarlierDeadlineAndCancelBothTimers(t *testing.T) {
	t.Parallel()
	deadline := time.Now().Add(time.Hour)
	req := &runtimev1.EpisodeRequest{Deadline: timestamppb.New(deadline), Budget: &runtimev1.EpisodeBudget{WallTime: durationpb.New(2 * time.Hour)}}
	ctx, cancel, err := boundedExecutionContext(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ctx.Deadline()
	if !ok || !got.Equal(deadline) {
		t.Fatalf("deadline = %v, want %v", got, deadline)
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("cancel = %v", ctx.Err())
	}
}

func TestInvalidWallBudgetCancelsExistingDeadline(t *testing.T) {
	t.Parallel()
	canceled := false
	req := &runtimev1.EpisodeRequest{Budget: &runtimev1.EpisodeBudget{WallTime: durationpb.New(0)}}
	ctx, cancel, err := applyWallBudget(t.Context(), func() { canceled = true }, req)
	if ctx != nil || cancel != nil || status.Code(err) != codes.InvalidArgument || !canceled {
		t.Fatalf("invalid budget: ctx=%v err=%v canceled=%v", ctx, err, canceled)
	}
}
