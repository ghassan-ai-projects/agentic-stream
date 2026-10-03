package evidence

import (
	"fmt"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// admitCall turns a wire request into an authorized, deadline-bound Call:
// the service is configured, the envelope is complete, the trace parses, the
// capability verifies, and the request stays inside the capability's scope.
func (s *Server) admitCall(req *runtimev1.EvidenceToolCall, now time.Time) (Call, Scope, error) {
	if err := s.checkService(req); err != nil {
		return Call{}, Scope{}, err
	}
	trace, scope, err := s.authenticateCall(req)
	if err != nil {
		return Call{}, Scope{}, err
	}
	call, err := s.authorizedCall(req, scope, trace)
	if err != nil {
		return Call{}, Scope{}, err
	}
	if call.Deadline, err = callDeadline(req, scope, now); err != nil {
		return Call{}, Scope{}, err
	}
	return call, scope, nil
}

func (s *Server) checkService(req *runtimev1.EvidenceToolCall) error {
	if req == nil || s.Verifier == nil || s.Query == nil {
		return wireError(codes.FailedPrecondition, "evidence service is not configured")
	}
	if s.RequireLedger && (s.Ledger == nil || s.RuntimeEpoch == "") {
		return wireError(codes.FailedPrecondition, "durable evidence ledger is required")
	}
	return validateCallEnvelope(req)
}

func (s *Server) authenticateCall(req *runtimev1.EvidenceToolCall) (contractsv1.TraceContext, Scope, error) { //nolint:wrapcheck // gRPC status is the wire contract.
	trace, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate())
	if err != nil {
		return contractsv1.TraceContext{}, Scope{}, wireError(codes.InvalidArgument, fmt.Sprintf("trace context: %v", err))
	} //nolint:wrapcheck // gRPC wire boundary.
	scope, err := s.Verifier.Verify(req.GetCapabilityToken())
	if err != nil {
		return contractsv1.TraceContext{}, Scope{}, wireError(codes.PermissionDenied, "capability authorization failed")
	}
	return trace, scope, nil
}
