// Package transport adapts EvidenceTools gRPC to the bounded evidence application.
package transport

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// Server exposes the worker protocol without authorization rules.
type Server struct {
	runtimev1.UnimplementedEvidenceToolsServer
	app *app.Service
}

// New binds an already configured application.
func New(service *app.Service) *Server { return &Server{app: service} }

// Call converts one worker envelope and its bounded result.
func (s *Server) Call(ctx context.Context, req *runtimev1.EvidenceToolCall) (*runtimev1.EvidenceToolResult, error) {
	if req == nil {
		return nil, status.Error(codes.FailedPrecondition, "evidence service is not configured") //nolint:wrapcheck // gRPC status is the wire contract.
	}
	result, err := s.app.Call(ctx, wire.DecodeEnvelope(req))
	if err != nil {
		return nil, statusError(err)
	}
	return wire.ResultMessage(req, result), nil
}
func statusError(err error) error {
	var refused *domain.Refusal
	if !errors.As(err, &refused) {
		return status.Error(codes.Internal, "evidence query failed") //nolint:wrapcheck // gRPC status is the wire contract.
	}
	return status.Error(refusalCode(refused.Kind), refused.Message) //nolint:wrapcheck // gRPC status is the wire contract.
}
func refusalCode(kind domain.ErrorKind) codes.Code {
	code, ok := map[domain.ErrorKind]codes.Code{domain.InvalidArgument: codes.InvalidArgument, domain.ResourceExhausted: codes.ResourceExhausted, domain.PermissionDenied: codes.PermissionDenied, domain.FailedPrecondition: codes.FailedPrecondition, domain.AlreadyExists: codes.AlreadyExists, domain.Internal: codes.Internal, domain.Canceled: codes.Canceled, domain.DeadlineExceeded: codes.DeadlineExceeded}[kind]
	if !ok {
		return codes.Internal
	}
	return code
}
