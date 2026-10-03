package evidence

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"

	"google.golang.org/grpc/codes"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
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
	return s.queryResult(ctx, req, call, reservation, now)
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
		return ledgerReservation{}, nil, s.reserveInMemory(call.CallID)
	}
	reservation, err := s.Ledger.Reserve(ctx, call, scope.TokenID, s.RuntimeEpoch)
	if err != nil {
		return ledgerReservation{}, nil, wireError(codes.FailedPrecondition, "evidence call reservation failed")
	}
	if reservation.Completed != nil {
		return reservation, resultMessage(req, *reservation.Completed), nil
	}
	if err := checkReservationCreated(reservation); err != nil {
		return ledgerReservation{}, nil, err
	}
	return reservation, nil, nil
}

func (s *Server) reserveInMemory(callID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls == nil {
		s.calls = make(map[string]struct{})
	}
	if _, exists := s.calls[callID]; exists {
		return wireError(codes.AlreadyExists, "evidence call was already used")
	}
	s.calls[callID] = struct{}{}
	return nil
}

func checkReservationCreated(reservation ledgerReservation) error {
	if reservation.Created {
		return nil
	}
	if reservation.Status != "running" {
		return wireError(codes.FailedPrecondition, "evidence call is terminal")
	}
	return wireError(codes.AlreadyExists, "evidence call is already in progress")
}

// runQuery invokes the query within the call deadline and enforces the result
// bounds. A failed call is recorded terminal in the ledger; without a ledger a
// failed query releases the call identity for a retry.
func (s *Server) runQuery(ctx context.Context, call Call, reservation ledgerReservation, now time.Time) (QueryResult, error) {
	queryContext, cancel := context.WithTimeout(ctx, call.Deadline.Sub(now))
	defer cancel()
	result, err := s.Query(queryContext, call)
	if err != nil {
		return QueryResult{}, s.queryFailure(ctx, queryContext, call, reservation)
	}
	if queryContext.Err() != nil {
		return QueryResult{}, contextStatusError(queryContext.Err())
	}
	if err := s.checkResultBounds(ctx, call, reservation, result); err != nil {
		return QueryResult{}, err
	}
	return result, nil
}

func (s *Server) queryFailure(ctx, queryContext context.Context, call Call, reservation ledgerReservation) error {
	s.failCall(ctx, reservation, "query_failed")
	if s.Ledger == nil {
		s.mu.Lock()
		delete(s.calls, call.CallID)
		s.mu.Unlock()
	}
	if queryContext.Err() != nil {
		return contextStatusError(queryContext.Err())
	}
	return wireError(codes.Internal, "evidence query failed") // Do not leak query details.
}

func (s *Server) checkResultBounds(ctx context.Context, call Call, reservation ledgerReservation, result QueryResult) error {
	if uint64(len(result.JSON)) > call.MaxBytes {
		s.failCall(ctx, reservation, "result_bytes_exceeded")
		return wireError(codes.ResourceExhausted, "evidence result exceeds capability")
	}
	if result.RowCount > call.MaxRows {
		s.failCall(ctx, reservation, "result_rows_exceeded")
		return wireError(codes.ResourceExhausted, "evidence result exceeds row limit")
	}
	return nil
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

func (s *Server) queryResult(ctx context.Context, req *runtimev1.EvidenceToolCall, call Call, reservation ledgerReservation, now time.Time) (*runtimev1.EvidenceToolResult, error) {
	result, err := s.runQuery(ctx, call, reservation, now)
	if err != nil {
		return nil, err
	}
	if err := s.commitResult(ctx, reservation, result); err != nil {
		return nil, err
	}
	return resultMessage(req, result), nil
}
