package worker

import (
	"context"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Execute validates the request, emits a Started event, and validates the
// complete worker stream. EOF without a terminal event is rejected.
func (s *Server) Execute(req *runtimev1.EpisodeRequest, stream runtimev1.EpisodeWorker_ExecuteServer) (err error) { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	defer func() {
		if recover() != nil {
			err = wireError(codes.Internal, "worker handler panic")
		}
	}()
	if err := s.validateRequest(req); err != nil {
		return err
	}
	if s.ExecuteFunc == nil {
		return wireError(codes.Unimplemented, "worker execute handler is not configured")
	}
	return s.executeStream(req, stream)
}

func (s *Server) executeStream(req *runtimev1.EpisodeRequest, stream runtimev1.EpisodeWorker_ExecuteServer) error {
	validator := streamValidator{episodeID: req.GetEpisodeId(), attemptID: req.GetAttemptId(), fence: req.GetFence(), maxEventBytes: s.limitsEvent(), maxEvents: s.limitsEvents(), maxStreamBytes: s.limitsStreamBytes()}
	if err := validator.emit(stream, s.started(req)); err != nil {
		return err
	}
	emit := func(event *runtimev1.EpisodeEvent) error {
		return validator.emit(stream, event)
	}
	executionContext, cancel, err := boundedExecutionContext(stream.Context(), req)
	if err != nil {
		return err
	}
	defer cancel()
	return s.concludeStream(executionContext, req, emit, &validator)
}

func (s *Server) concludeStream(executionContext context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error, validator *streamValidator) error {
	if err := s.executeHandler(executionContext, req, emit); err != nil {
		return err
	}
	if err := executionContext.Err(); err != nil {
		return wireError(codes.DeadlineExceeded, "episode execution deadline exceeded")
	}
	if !validator.terminal {
		return wireError(codes.FailedPrecondition, "worker stream ended without terminal event")
	}
	return nil
}

func (s *Server) executeHandler(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error { //nolint:wrapcheck // Preserve handler gRPC status errors at the wire boundary.
	if err := s.ExecuteFunc(ctx, req, emit); err != nil {
		if status.Code(err) == codes.Canceled || status.Code(err) == codes.DeadlineExceeded {
			return err
		}
		if status.Code(err) != codes.Unknown {
			return err
		}
		return wireErrorf(codes.Internal, "worker execution failed: %v", err)
	}
	return nil
}

func boundedExecutionContext(ctx context.Context, req *runtimev1.EpisodeRequest) (context.Context, context.CancelFunc, error) {
	executionContext := ctx
	cancel := context.CancelFunc(func() {})
	if deadline := req.GetDeadline(); deadline != nil {
		executionContext, cancel = context.WithDeadline(executionContext, deadline.AsTime())
	}
	return applyWallBudget(executionContext, cancel, req)
}

func applyWallBudget(executionContext context.Context, cancel context.CancelFunc, req *runtimev1.EpisodeRequest) (context.Context, context.CancelFunc, error) {
	if budget := req.GetBudget(); budget != nil && budget.GetWallTime() != nil {
		wallTime := budget.GetWallTime().AsDuration()
		if wallTime <= 0 {
			cancel()
			return nil, nil, wireError(codes.InvalidArgument, "wall_time budget must be positive")
		}
		deadlineCancel := cancel
		var budgetCancel context.CancelFunc
		executionContext, budgetCancel = context.WithTimeout(executionContext, wallTime)
		cancel = func() { budgetCancel(); deadlineCancel() }
	}
	return executionContext, cancel, nil
}
