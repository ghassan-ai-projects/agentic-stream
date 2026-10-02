package evidence

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxArgumentsBytes = 256 << 10
const maxCapabilityTokenBytes = 64 << 10

func wireError(code codes.Code, message string) error { return status.Error(code, message) } //nolint:wrapcheck // gRPC boundary.

// Query receives an already-authorized, bounded evidence request. It must not
// expose database handles, credentials, shell access, arbitrary HTTP, or
// effectors to the worker.
type Query func(context.Context, Call) (QueryResult, error)

// QueryResult is the bounded, typed output of a runtime-owned evidence
// provider.
type QueryResult struct {
	JSON     []byte
	RowCount uint64
}

// EvidenceGetArguments is the complete v1 schema for evidence.get. Query
// dimensions and limits are authenticated protobuf fields, not worker-owned
// JSON fields.
type EvidenceGetArguments struct {
	EntityID string `json:"entity_id"`
}

// Call is the validated application form of an EvidenceToolCall.
type Call struct {
	EpisodeID        string
	CallID           string
	ToolName         string
	TenantID         string
	SituationID      string
	SituationVersion int64
	EntityID         string
	Arguments        EvidenceGetArguments
	Deadline         time.Time
	AttemptID        string
	Fence            int64
	Trace            contractsv1.TraceContext
	MaxRows          uint64
	MaxBytes         uint64
	From             time.Time
	Until            time.Time
}

// Server implements the runtime-owned EvidenceTools service.
type Server struct {
	runtimev1.UnimplementedEvidenceToolsServer
	Verifier      *Verifier
	Query         Query
	Now           func() time.Time
	Ledger        *Ledger
	RuntimeEpoch  string
	RequireLedger bool
	mu            sync.Mutex
	calls         map[string]struct{}
}

// Call validates identity, trace context, capability scope, argument bounds,
// and result bounds before invoking the narrow query callback.
func (s *Server) Call(ctx context.Context, req *runtimev1.EvidenceToolCall) (*runtimev1.EvidenceToolResult, error) { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	now := s.now()
	call, scope, err := s.admitCall(req, now)
	if err != nil {
		return nil, err
	}
	reservation, replayed, err := s.reserve(ctx, req, call, scope)
	if err != nil || replayed != nil {
		return replayed, err
	}
	result, err := s.runQuery(ctx, call, reservation, now)
	if err != nil {
		return nil, err
	}
	if err := s.commitResult(ctx, reservation, result); err != nil {
		return nil, err
	}
	return resultMessage(req, result), nil
}

// admitCall turns a wire request into an authorized, deadline-bound Call:
// the service is configured, the envelope is complete, the trace parses, the
// capability verifies, and the request stays inside the capability's scope.
func (s *Server) admitCall(req *runtimev1.EvidenceToolCall, now time.Time) (Call, Scope, error) { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	if req == nil || s.Verifier == nil || s.Query == nil {
		return Call{}, Scope{}, status.Error(codes.FailedPrecondition, "evidence service is not configured") //nolint:wrapcheck // gRPC wire boundary.
	}
	if s.RequireLedger && (s.Ledger == nil || s.RuntimeEpoch == "") {
		return Call{}, Scope{}, wireError(codes.FailedPrecondition, "durable evidence ledger is required")
	}
	if err := validateCallEnvelope(req); err != nil {
		return Call{}, Scope{}, err
	}
	trace, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate())
	if err != nil {
		return Call{}, Scope{}, status.Errorf(codes.InvalidArgument, "trace context: %v", err) //nolint:wrapcheck // gRPC wire boundary.
	}
	scope, err := s.Verifier.Verify(req.GetCapabilityToken())
	if err != nil {
		return Call{}, Scope{}, status.Error(codes.PermissionDenied, "capability authorization failed") //nolint:wrapcheck // Do not reveal token failure details.
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

// commitResult stores the result in the durable ledger when one is
// configured.
func (s *Server) commitResult(ctx context.Context, reservation ledgerReservation, result QueryResult) error {
	if s.Ledger == nil {
		return nil
	}
	if err := s.Ledger.Complete(ctx, reservation, result); err != nil {
		return wireError(codes.FailedPrecondition, "evidence result could not be committed")
	}
	return nil
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// reserve claims the call identity, durably when a ledger is configured. A
// completed durable call returns its stored result instead of re-querying.
func (s *Server) reserve(ctx context.Context, req *runtimev1.EvidenceToolCall, call Call, scope Scope) (ledgerReservation, *runtimev1.EvidenceToolResult, error) {
	if s.Ledger == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.calls == nil {
			s.calls = make(map[string]struct{})
		}
		if _, exists := s.calls[call.CallID]; exists {
			return ledgerReservation{}, nil, wireError(codes.AlreadyExists, "evidence call was already used")
		}
		s.calls[call.CallID] = struct{}{}
		return ledgerReservation{}, nil, nil
	}
	reservation, err := s.Ledger.Reserve(ctx, call, scope.TokenID, s.RuntimeEpoch)
	if err != nil {
		return ledgerReservation{}, nil, wireError(codes.FailedPrecondition, "evidence call reservation failed")
	}
	if reservation.Completed != nil {
		return reservation, resultMessage(req, *reservation.Completed), nil
	}
	if !reservation.Created {
		if reservation.Status != "running" {
			return ledgerReservation{}, nil, wireError(codes.FailedPrecondition, "evidence call is terminal")
		}
		return ledgerReservation{}, nil, wireError(codes.AlreadyExists, "evidence call is already in progress")
	}
	return reservation, nil, nil
}

// runQuery invokes the query within the call deadline and enforces the result
// bounds. A failed call is recorded terminal in the ledger; without a ledger a
// failed query releases the call identity for a retry.
func (s *Server) runQuery(ctx context.Context, call Call, reservation ledgerReservation, now time.Time) (QueryResult, error) {
	queryContext, cancel := context.WithTimeout(ctx, call.Deadline.Sub(now))
	defer cancel()
	result, err := s.Query(queryContext, call)
	if err != nil {
		s.failCall(ctx, reservation, "query_failed")
		if s.Ledger == nil {
			s.mu.Lock()
			delete(s.calls, call.CallID)
			s.mu.Unlock()
		}
		if queryContext.Err() != nil {
			return QueryResult{}, contextStatusError(queryContext.Err())
		}
		return QueryResult{}, wireError(codes.Internal, "evidence query failed") // Do not leak query details.
	}
	if queryContext.Err() != nil {
		return QueryResult{}, contextStatusError(queryContext.Err())
	}
	if uint64(len(result.JSON)) > call.MaxBytes {
		s.failCall(ctx, reservation, "result_bytes_exceeded")
		return QueryResult{}, wireError(codes.ResourceExhausted, "evidence result exceeds capability")
	}
	if result.RowCount > call.MaxRows {
		s.failCall(ctx, reservation, "result_rows_exceeded")
		return QueryResult{}, wireError(codes.ResourceExhausted, "evidence result exceeds row limit")
	}
	return result, nil
}

// failCall records a terminal failure when a durable ledger is configured.
// The wire error the caller returns is the authoritative failure signal, so
// a failed ledger write is not surfaced separately.
func (s *Server) failCall(ctx context.Context, reservation ledgerReservation, code string) {
	if s.Ledger != nil {
		_ = s.Ledger.Fail(ctx, reservation, code)
	}
}

func resultMessage(req *runtimev1.EvidenceToolCall, result QueryResult) *runtimev1.EvidenceToolResult {
	hash := sha256.Sum256(result.JSON)
	return resultMessageWithHash(req, result, hash)
}

func resultMessageWithHash(req *runtimev1.EvidenceToolCall, result QueryResult, hash [sha256.Size]byte) *runtimev1.EvidenceToolResult {
	return &runtimev1.EvidenceToolResult{EpisodeId: req.GetEpisodeId(), CallId: req.GetCallId(), ResultJson: result.JSON, ResultSha256: hash[:], ResultBytes: uint64(len(result.JSON)), RowCount: result.RowCount, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), Traceparent: req.GetTraceparent(), Tracestate: req.GetTracestate()}
}

func contextStatusError(err error) error {
	if errors.Is(err, context.Canceled) {
		return wireError(codes.Canceled, "evidence query canceled")
	}
	return wireError(codes.DeadlineExceeded, "evidence query deadline exceeded")
}
