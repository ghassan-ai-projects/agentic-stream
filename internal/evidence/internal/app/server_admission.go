package app

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
)

func (s *Server) admitCall(req domain.Envelope, now time.Time) (Call, Scope, error) {
	if err := domain.ValidateEnvelope(req); err != nil {
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
	if call.Deadline, err = domain.CallDeadline(req, scope, now); err != nil {
		return Call{}, Scope{}, err
	}
	return call, scope, nil
}
func (s *Server) authenticateCall(req domain.Envelope) (contractsv1.TraceContext, Scope, error) {
	trace, err := contractsv1.ParseTraceContext(req.Traceparent, req.Tracestate)
	if err != nil {
		return contractsv1.TraceContext{}, Scope{}, domain.Refuse(domain.InvalidArgument, fmt.Sprintf("trace context: %v", err))
	}
	scope, err := s.Verifier.Verify(req.CapabilityToken)
	if err != nil {
		return contractsv1.TraceContext{}, Scope{}, domain.Refuse(domain.PermissionDenied, "capability authorization failed")
	}
	return trace, scope, nil
}
func (s *Server) authorizedCall(req domain.Envelope, scope Scope, trace contractsv1.TraceContext) (Call, error) {
	if err := domain.AuthorizeScope(req, scope, s.RuntimeEpoch); err != nil {
		return Call{}, err
	}
	arguments, err := wire.DecodeEvidenceGetArguments(req.ArgumentsJSON)
	if err != nil {
		return Call{}, domain.Refuse(domain.InvalidArgument, "invalid evidence.get arguments")
	}
	return domain.BindArguments(req, scope, trace, arguments)
}
