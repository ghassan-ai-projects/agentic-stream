package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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
	if req == nil || s.Verifier == nil || s.Query == nil {
		return nil, status.Error(codes.FailedPrecondition, "evidence service is not configured") //nolint:wrapcheck // gRPC wire boundary.
	}
	if s.RequireLedger && (s.Ledger == nil || s.RuntimeEpoch == "") {
		return nil, wireError(codes.FailedPrecondition, "durable evidence ledger is required")
	}
	if req.GetProtocolVersion() != "1.0" || req.GetEpisodeId() == "" || req.GetCallId() == "" || req.GetToolName() == "" || req.GetTenantId() == "" || req.GetSituationId() == "" || req.GetEntityId() == "" || req.GetAttemptId() == "" || req.GetFence() == 0 || req.GetMaxRows() == 0 || req.GetMaxBytes() == 0 {
		return nil, status.Error(codes.InvalidArgument, "evidence call identity is incomplete") //nolint:wrapcheck // gRPC wire boundary.
	}
	if len(req.GetArgumentsJson()) == 0 || len(req.GetArgumentsJson()) > maxArgumentsBytes || len(req.GetCapabilityToken()) > maxCapabilityTokenBytes || proto.Size(req) > maxArgumentsBytes+maxCapabilityTokenBytes {
		return nil, status.Error(codes.ResourceExhausted, "evidence arguments exceed limits") //nolint:wrapcheck // gRPC wire boundary.
	}
	trace, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "trace context: %v", err) //nolint:wrapcheck // gRPC wire boundary.
	}
	scope, err := s.Verifier.Verify(req.GetCapabilityToken())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "capability authorization failed") //nolint:wrapcheck // Do not reveal token failure details.
	}
	if req.GetFence() > uint64(^uint64(0)>>1) {
		return nil, wireError(codes.InvalidArgument, "fence exceeds runtime range")
	}
	if req.GetSituationVersion() > uint64(^uint64(0)>>1) {
		return nil, wireError(codes.InvalidArgument, "situation version exceeds runtime range")
	}
	fence := int64(req.GetFence())                       //nolint:gosec // Checked against MaxInt64 immediately above.
	situationVersion := int64(req.GetSituationVersion()) //nolint:gosec // Checked against MaxInt64 immediately above.
	if scope.EpisodeID != req.GetEpisodeId() || scope.AttemptID != req.GetAttemptId() || scope.Fence != fence || scope.TenantID != req.GetTenantId() || scope.SituationID != req.GetSituationId() || scope.SituationVersion != situationVersion || scope.EntityID != req.GetEntityId() || !contains(scope.Tools, req.GetToolName()) || scope.Traceparent != req.GetTraceparent() || scope.Tracestate != req.GetTracestate() || (s.RuntimeEpoch != "" && scope.RuntimeEpoch != s.RuntimeEpoch) {
		return nil, status.Error(codes.PermissionDenied, "capability scope mismatch") //nolint:wrapcheck // gRPC wire boundary.
	}
	if req.GetTimeFrom() == nil || req.GetTimeUntil() == nil || !req.GetTimeFrom().IsValid() || !req.GetTimeUntil().IsValid() || req.GetTimeUntil().AsTime().Before(req.GetTimeFrom().AsTime()) || req.GetTimeFrom().AsTime().Before(scope.From) || req.GetTimeUntil().AsTime().After(scope.Until) {
		return nil, status.Error(codes.PermissionDenied, "evidence time range exceeds capability") //nolint:wrapcheck // gRPC wire boundary.
	}
	arguments, err := decodeEvidenceGetArguments(req.GetArgumentsJson())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid evidence.get arguments") //nolint:wrapcheck // gRPC wire boundary.
	}
	if arguments.EntityID != scope.EntityID || arguments.EntityID != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied, "entity scope mismatch") //nolint:wrapcheck // gRPC wire boundary.
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	deadline := now.Add(time.Minute)
	if req.GetDeadline() != nil {
		if !req.GetDeadline().IsValid() {
			return nil, wireError(codes.InvalidArgument, "deadline is invalid")
		} //nolint:wrapcheck // gRPC wire boundary.
		deadline = req.GetDeadline().AsTime()
	}
	if !deadline.Before(scope.ExpiresAt) {
		deadline = scope.ExpiresAt
	}
	if !deadline.After(now) {
		return nil, wireError(codes.DeadlineExceeded, "evidence call deadline has expired")
	}
	call := Call{EpisodeID: req.GetEpisodeId(), CallID: req.GetCallId(), ToolName: req.GetToolName(), TenantID: req.GetTenantId(), SituationID: req.GetSituationId(), SituationVersion: situationVersion, EntityID: arguments.EntityID, Arguments: arguments, Deadline: deadline, AttemptID: req.GetAttemptId(), Fence: fence, Trace: trace, MaxRows: minNonZero(req.GetMaxRows(), scope.MaxRows), MaxBytes: minNonZero(req.GetMaxBytes(), scope.MaxBytes), From: req.GetTimeFrom().AsTime(), Until: req.GetTimeUntil().AsTime()}
	var reservation ledgerReservation
	if s.Ledger != nil {
		reservation, err = s.Ledger.Reserve(ctx, call, scope.TokenID, s.RuntimeEpoch)
		if err != nil {
			return nil, wireError(codes.FailedPrecondition, "evidence call reservation failed")
		}
		if reservation.Completed != nil {
			return resultMessage(req, *reservation.Completed), nil
		}
		if !reservation.Created {
			if reservation.Status != "running" {
				return nil, wireError(codes.FailedPrecondition, "evidence call is terminal")
			}
			return nil, wireError(codes.AlreadyExists, "evidence call is already in progress")
		}
	} else {
		s.mu.Lock()
		if s.calls == nil {
			s.calls = make(map[string]struct{})
		}
		if _, exists := s.calls[call.CallID]; exists {
			s.mu.Unlock()
			return nil, wireError(codes.AlreadyExists, "evidence call was already used")
		}
		s.calls[call.CallID] = struct{}{}
		s.mu.Unlock()
	}
	queryContext, cancel := context.WithTimeout(ctx, deadline.Sub(now))
	defer cancel()
	result, err := s.Query(queryContext, call)
	if err != nil {
		if s.Ledger != nil {
			_ = s.Ledger.Fail(context.Background(), reservation, "query_failed")
		}
		if s.Ledger == nil {
			s.mu.Lock()
			delete(s.calls, call.CallID)
			s.mu.Unlock()
		}
		if queryContext.Err() != nil {
			return nil, contextStatusError(queryContext.Err())
		}
		return nil, wireError(codes.Internal, "evidence query failed")
	} //nolint:wrapcheck // Do not leak query details.
	if queryContext.Err() != nil {
		return nil, contextStatusError(queryContext.Err())
	}
	if uint64(len(result.JSON)) > call.MaxBytes {
		if s.Ledger != nil {
			_ = s.Ledger.Fail(context.Background(), reservation, "result_bytes_exceeded")
		}
		return nil, wireError(codes.ResourceExhausted, "evidence result exceeds capability")
	} //nolint:wrapcheck // gRPC wire boundary.
	if result.RowCount > call.MaxRows {
		if s.Ledger != nil {
			_ = s.Ledger.Fail(context.Background(), reservation, "result_rows_exceeded")
		}
		return nil, wireError(codes.ResourceExhausted, "evidence result exceeds row limit")
	}
	if s.Ledger != nil {
		if err := s.Ledger.Complete(ctx, reservation, result); err != nil {
			return nil, wireError(codes.FailedPrecondition, "evidence result could not be committed")
		}
	}
	hash := sha256.Sum256(result.JSON)
	return resultMessageWithHash(req, result, hash), nil
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

func decodeEvidenceGetArguments(raw []byte) (EvidenceGetArguments, error) {
	var arguments EvidenceGetArguments
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil || arguments.EntityID == "" {
		return EvidenceGetArguments{}, fmt.Errorf("decode evidence.get arguments")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return EvidenceGetArguments{}, fmt.Errorf("evidence.get arguments contain trailing data")
	}
	return arguments, nil
}

func minNonZero(left, right uint64) uint64 {
	if left == 0 {
		return right
	}
	if left < right {
		return left
	}
	return right
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
