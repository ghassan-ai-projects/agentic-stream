// Package worker implements the runtime-facing side of the versioned episode
// worker protocol. It owns wire validation and event sequencing; a worker
// handler only supplies bounded, typed proposals.
package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ProtocolVersion = "1.0"
	ContractVersion = "1.0"

	DefaultMaxRequestBytes = 4 << 20
	DefaultMaxEventBytes   = 1 << 20
	DefaultMaxEvents       = 4096
	DefaultMaxStreamBytes  = 16 << 20
)

// wireError preserves gRPC status codes while keeping the wire boundary's
// deliberate status construction out of application error wrapping rules.
func wireError(code codes.Code, message string) error {
	return status.Error(code, message) //nolint:wrapcheck // This is the gRPC wire boundary.
}

func wireErrorf(code codes.Code, format string, args ...any) error {
	return status.Errorf(code, format, args...) //nolint:wrapcheck // This is the gRPC wire boundary.
}

// ExecuteFunc emits worker-originated events after the server has emitted the
// initial EpisodeStarted event. The function must emit exactly one Terminal
// event. The runtime validates every emitted event before it reaches the wire.
type ExecuteFunc func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error

// Server is a validating EpisodeWorker implementation. It deliberately has no
// access to effectors, credentials, persistence, or unbounded tool handles.
type Server struct {
	runtimev1.UnimplementedEpisodeWorkerServer

	WorkerName        string
	WorkerVersion     string
	SupportedFeatures []string
	MaxRequestBytes   uint64
	MaxEventBytes     uint64
	MaxEvents         uint64
	MaxStreamBytes    uint64
	Now               func() time.Time
	ExecuteFunc       ExecuteFunc
}

func (s *Server) limits() (uint64, uint64) {
	request, event := s.MaxRequestBytes, s.MaxEventBytes
	if request == 0 {
		request = DefaultMaxRequestBytes
	}
	if event == 0 {
		event = DefaultMaxEventBytes
	}
	return request, event
}

// Handshake accepts only the current protocol and contract versions and
// rejects required features that this worker does not advertise.
func (s *Server) Handshake(_ context.Context, req *runtimev1.HandshakeRequest) (*runtimev1.HandshakeResponse, error) { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	if req == nil || req.GetWorkerId() == "" || req.GetRuntimeInstanceId() == "" {
		return nil, wireError(codes.InvalidArgument, "worker_id and runtime_instance_id are required")
	}
	if req.GetProtocolVersion() != ProtocolVersion {
		return nil, wireErrorf(codes.FailedPrecondition, "unsupported protocol version %q", req.GetProtocolVersion())
	}
	if req.GetContractVersion() != ContractVersion {
		return nil, wireErrorf(codes.FailedPrecondition, "unsupported contract version %q", req.GetContractVersion())
	}
	features := make(map[string]struct{}, len(s.SupportedFeatures))
	for _, feature := range s.SupportedFeatures {
		features[feature] = struct{}{}
	}
	for _, requested := range req.GetRequestedFeatures() {
		if _, ok := features[requested]; !ok {
			return nil, wireErrorf(codes.FailedPrecondition, "unsupported required feature %q", requested)
		}
	}
	maxRequest, maxEvent := s.limits()
	return &runtimev1.HandshakeResponse{
		ProtocolVersion:   ProtocolVersion,
		ContractVersion:   ContractVersion,
		WorkerName:        s.WorkerName,
		WorkerVersion:     s.WorkerVersion,
		SupportedFeatures: append([]string(nil), s.SupportedFeatures...),
		MaxRequestBytes:   maxRequest,
		MaxEventBytes:     maxEvent,
	}, nil
}

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
	validator := streamValidator{episodeID: req.GetEpisodeId(), attemptID: req.GetAttemptId(), fence: req.GetFence(), maxEventBytes: s.limitsEvent(), maxEvents: s.limitsEvents(), maxStreamBytes: s.limitsStreamBytes()}
	if err := validator.emit(stream, s.started(req)); err != nil {
		return err
	}
	emit := func(event *runtimev1.EpisodeEvent) error {
		return validator.emit(stream, event)
	}
	executionContext := stream.Context()
	if deadline := req.GetDeadline(); deadline != nil {
		var cancel context.CancelFunc
		executionContext, cancel = context.WithDeadline(executionContext, deadline.AsTime())
		defer cancel()
	}
	if budget := req.GetBudget(); budget != nil && budget.GetWallTime() != nil {
		wallTime := budget.GetWallTime().AsDuration()
		if wallTime <= 0 {
			return wireError(codes.InvalidArgument, "wall_time budget must be positive")
		}
		var cancel context.CancelFunc
		executionContext, cancel = context.WithTimeout(executionContext, wallTime)
		defer cancel()
	}
	if err := s.ExecuteFunc(executionContext, req, emit); err != nil {
		if status.Code(err) == codes.Canceled || status.Code(err) == codes.DeadlineExceeded {
			return err
		}
		if status.Code(err) != codes.Unknown {
			return err
		}
		return wireErrorf(codes.Internal, "worker execution failed: %v", err)
	}
	if err := executionContext.Err(); err != nil {
		return wireError(codes.DeadlineExceeded, "episode execution deadline exceeded")
	}
	if !validator.terminal {
		return wireError(codes.FailedPrecondition, "worker stream ended without terminal event")
	}
	return nil
}

func (s *Server) limitsEvent() uint64 {
	_, event := s.limits()
	return event
}

func (s *Server) limitsEvents() uint64 {
	if s.MaxEvents == 0 {
		return DefaultMaxEvents
	}
	return s.MaxEvents
}

func (s *Server) limitsStreamBytes() uint64 {
	if s.MaxStreamBytes == 0 {
		return DefaultMaxStreamBytes
	}
	return s.MaxStreamBytes
}

func (s *Server) started(req *runtimev1.EpisodeRequest) *runtimev1.EpisodeEvent {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return &runtimev1.EpisodeEvent{
		EpisodeId:  req.GetEpisodeId(),
		Sequence:   1,
		OccurredAt: timestamppb.New(now),
		AttemptId:  req.GetAttemptId(),
		Fence:      req.GetFence(),
		Payload: &runtimev1.EpisodeEvent_Started{Started: &runtimev1.EpisodeStarted{
			WorkerName: s.WorkerName, WorkerVersion: s.WorkerVersion,
		}},
	}
}

func (s *Server) validateRequest(req *runtimev1.EpisodeRequest) error { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	if req == nil {
		return wireError(codes.InvalidArgument, "episode request is required")
	}
	maxRequest, _ := s.limits()
	if uint64(proto.Size(req)) > maxRequest { //nolint:gosec // protobuf Size is non-negative and bounded by the configured request limit.
		return wireError(codes.ResourceExhausted, "episode request exceeds size limit")
	}
	if !sameMajor(req.GetProtocolVersion(), ProtocolVersion) {
		return wireErrorf(codes.FailedPrecondition, "unsupported protocol version %q", req.GetProtocolVersion())
	}
	if req.GetEvidenceToolsEndpoint() != "" || len(req.GetCapabilityToken()) != 0 {
		return wireError(codes.FailedPrecondition, "evidence tools are not enabled in this worker phase")
	}
	for name, value := range map[string]string{
		"episode_id": req.GetEpisodeId(), "tenant_id": req.GetTenantId(),
		"situation_id": req.GetSituationId(), "attempt_id": req.GetAttemptId(),
	} {
		if strings.TrimSpace(value) == "" {
			return wireErrorf(codes.InvalidArgument, "%s is required", name)
		}
	}
	if req.GetFence() == 0 || req.GetSituationVersion() == 0 {
		return wireError(codes.InvalidArgument, "situation_version and fence must be positive")
	}
	if req.GetKind() == runtimev1.EpisodeKind_EPISODE_KIND_UNSPECIFIED || req.GetLane() == runtimev1.EpisodeLane_EPISODE_LANE_UNSPECIFIED || req.GetRiskCeiling() == runtimev1.RiskClass_RISK_CLASS_UNSPECIFIED {
		return wireError(codes.InvalidArgument, "kind, lane, and risk_ceiling are required")
	}
	if len(req.GetSnapshotSha256()) != 32 || len(req.GetSpecSha256()) != 32 {
		return wireError(codes.InvalidArgument, "snapshot_sha256 and spec_sha256 must be 32 bytes")
	}
	if len(req.GetSnapshotJson()) == 0 || len(req.GetDecisionSchemaJson()) == 0 || len(req.GetToolCatalogJson()) == 0 {
		return wireError(codes.InvalidArgument, "snapshot, decision schema, and tool catalog are required")
	}
	if _, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate()); err != nil {
		return wireErrorf(codes.InvalidArgument, "trace context: %v", err)
	}
	if req.GetDeadline() != nil {
		if !req.GetDeadline().IsValid() {
			return wireError(codes.InvalidArgument, "deadline is invalid")
		}
		if req.GetDeadline().AsTime().Before(time.Now().UTC()) {
			return wireError(codes.DeadlineExceeded, "episode deadline has expired")
		}
	}
	return nil
}

func sameMajor(got, want string) bool {
	return strings.TrimSpace(got) == strings.TrimSpace(want)
}

type streamValidator struct {
	episodeID      string
	attemptID      string
	fence          uint64
	maxEventBytes  uint64
	maxEvents      uint64
	maxStreamBytes uint64
	nextSequence   uint64
	eventCount     uint64
	streamBytes    uint64
	terminal       bool
	mu             sync.Mutex
}

func (v *streamValidator) emit(stream runtimev1.EpisodeWorker_ExecuteServer, event *runtimev1.EpisodeEvent) error { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	v.mu.Lock()
	defer v.mu.Unlock()
	if event == nil {
		return wireError(codes.InvalidArgument, "nil episode event")
	}
	if uint64(proto.Size(event)) > v.maxEventBytes { //nolint:gosec // protobuf Size is non-negative and bounded by the configured event limit.
		return wireError(codes.ResourceExhausted, "episode event exceeds size limit")
	}
	eventBytes := uint64(proto.Size(event)) //nolint:gosec // protobuf Size is non-negative and bounded by the configured event limit.
	if v.eventCount >= v.maxEvents || v.streamBytes+eventBytes > v.maxStreamBytes {
		return wireError(codes.ResourceExhausted, "episode stream exceeds size limit")
	}
	if v.terminal {
		return wireError(codes.FailedPrecondition, "event emitted after terminal")
	}
	if event.GetEpisodeId() != v.episodeID || event.GetAttemptId() != v.attemptID || event.GetFence() != v.fence {
		return wireError(codes.PermissionDenied, "episode event identity does not match request")
	}
	if event.GetSequence() == 0 || event.GetSequence() != v.nextSequence+1 {
		return wireErrorf(codes.FailedPrecondition, "episode event sequence %d is not %d", event.GetSequence(), v.nextSequence+1)
	}
	if event.GetOccurredAt() == nil || !event.GetOccurredAt().IsValid() || event.GetPayload() == nil {
		return wireError(codes.InvalidArgument, "episode event timestamp and payload are required")
	}
	if decision := event.GetDecision(); decision != nil {
		if decision.GetEpisodeId() != v.episodeID || decision.GetAttemptId() != v.attemptID || decision.GetFence() != v.fence || len(decision.GetDecisionJson()) == 0 || len(decision.GetDecisionSha256()) != 32 {
			return wireError(codes.PermissionDenied, "decision identity or digest is invalid")
		}
	}
	if terminal := event.GetTerminal(); terminal != nil {
		if terminal.GetStatus() == runtimev1.TerminalStatus_TERMINAL_STATUS_UNSPECIFIED {
			return wireError(codes.InvalidArgument, "terminal status is required")
		}
		v.terminal = true
	}
	v.nextSequence = event.GetSequence()
	v.eventCount++
	v.streamBytes += eventBytes
	if err := stream.Send(event); err != nil {
		return fmt.Errorf("send episode event: %w", err)
	}
	return nil
}
