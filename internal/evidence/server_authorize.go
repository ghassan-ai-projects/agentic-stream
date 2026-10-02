package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"io"
	"time"
)

// validateCallEnvelope requires a complete call identity within size limits.
func validateCallEnvelope(req *runtimev1.EvidenceToolCall) error {
	if req.GetProtocolVersion() != "1.0" || req.GetEpisodeId() == "" || req.GetCallId() == "" || req.GetToolName() == "" || req.GetTenantId() == "" || req.GetSituationId() == "" || req.GetEntityId() == "" || req.GetAttemptId() == "" || req.GetFence() == 0 || req.GetMaxRows() == 0 || req.GetMaxBytes() == 0 {
		return status.Error(codes.InvalidArgument, "evidence call identity is incomplete") //nolint:wrapcheck // gRPC wire boundary.
	}
	if len(req.GetArgumentsJson()) == 0 || len(req.GetArgumentsJson()) > maxArgumentsBytes || len(req.GetCapabilityToken()) > maxCapabilityTokenBytes || proto.Size(req) > maxArgumentsBytes+maxCapabilityTokenBytes {
		return status.Error(codes.ResourceExhausted, "evidence arguments exceed limits") //nolint:wrapcheck // gRPC wire boundary.
	}
	return nil
}

// authorizedCall binds the request to the verified capability scope: every
// identity field, the tool, the trace, the runtime epoch, the time range, and
// the entity must match what the capability grants.
func (s *Server) authorizedCall(req *runtimev1.EvidenceToolCall, scope Scope, trace contractsv1.TraceContext) (Call, error) {
	if req.GetFence() > uint64(^uint64(0)>>1) {
		return Call{}, wireError(codes.InvalidArgument, "fence exceeds runtime range")
	}
	if req.GetSituationVersion() > uint64(^uint64(0)>>1) {
		return Call{}, wireError(codes.InvalidArgument, "situation version exceeds runtime range")
	}
	fence := int64(req.GetFence())                       //nolint:gosec // Checked against MaxInt64 immediately above.
	situationVersion := int64(req.GetSituationVersion()) //nolint:gosec // Checked against MaxInt64 immediately above.
	if !scope.grants(req, fence, situationVersion) || (s.RuntimeEpoch != "" && scope.RuntimeEpoch != s.RuntimeEpoch) {
		return Call{}, status.Error(codes.PermissionDenied, "capability scope mismatch") //nolint:wrapcheck // gRPC wire boundary.
	}
	if !scope.coversTimeRange(req) {
		return Call{}, status.Error(codes.PermissionDenied, "evidence time range exceeds capability") //nolint:wrapcheck // gRPC wire boundary.
	}
	arguments, err := decodeEvidenceGetArguments(req.GetArgumentsJson())
	if err != nil {
		return Call{}, status.Error(codes.InvalidArgument, "invalid evidence.get arguments") //nolint:wrapcheck // gRPC wire boundary.
	}
	if arguments.EntityID != scope.EntityID || arguments.EntityID != req.GetEntityId() {
		return Call{}, status.Error(codes.PermissionDenied, "entity scope mismatch") //nolint:wrapcheck // gRPC wire boundary.
	}
	return Call{EpisodeID: req.GetEpisodeId(), CallID: req.GetCallId(), ToolName: req.GetToolName(), TenantID: req.GetTenantId(), SituationID: req.GetSituationId(), SituationVersion: situationVersion, EntityID: arguments.EntityID, Arguments: arguments, AttemptID: req.GetAttemptId(), Fence: fence, Trace: trace, MaxRows: minNonZero(req.GetMaxRows(), scope.MaxRows), MaxBytes: minNonZero(req.GetMaxBytes(), scope.MaxBytes), From: req.GetTimeFrom().AsTime(), Until: req.GetTimeUntil().AsTime()}, nil
}

// grants reports whether the capability names exactly this call's attempt,
// situation version, entity, tool, and trace.
func (scope Scope) grants(req *runtimev1.EvidenceToolCall, fence, situationVersion int64) bool {
	return scope.EpisodeID == req.GetEpisodeId() && scope.AttemptID == req.GetAttemptId() && scope.Fence == fence &&
		scope.TenantID == req.GetTenantId() && scope.SituationID == req.GetSituationId() &&
		scope.SituationVersion == situationVersion && scope.EntityID == req.GetEntityId() &&
		contains(scope.Tools, req.GetToolName()) &&
		scope.Traceparent == req.GetTraceparent() && scope.Tracestate == req.GetTracestate()
}

// coversTimeRange reports whether the call asks for a valid, ordered time
// range inside the capability's evidence range.
func (scope Scope) coversTimeRange(req *runtimev1.EvidenceToolCall) bool {
	from, until := req.GetTimeFrom(), req.GetTimeUntil()
	if from == nil || until == nil || !from.IsValid() || !until.IsValid() {
		return false
	}
	return !until.AsTime().Before(from.AsTime()) && !from.AsTime().Before(scope.From) && !until.AsTime().After(scope.Until)
}

// callDeadline is the requested deadline (one minute by default), capped at
// the capability expiry, and must still be in the future.
func callDeadline(req *runtimev1.EvidenceToolCall, scope Scope, now time.Time) (time.Time, error) {
	deadline := now.Add(time.Minute)
	if req.GetDeadline() != nil {
		if !req.GetDeadline().IsValid() {
			return time.Time{}, wireError(codes.InvalidArgument, "deadline is invalid")
		}
		deadline = req.GetDeadline().AsTime()
	}
	if !deadline.Before(scope.ExpiresAt) {
		deadline = scope.ExpiresAt
	}
	if !deadline.After(now) {
		return time.Time{}, wireError(codes.DeadlineExceeded, "evidence call deadline has expired")
	}
	return deadline, nil
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
