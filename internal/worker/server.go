// Package worker implements the runtime-facing side of the versioned episode
// worker protocol. It owns wire validation and event sequencing; a worker
// handler only supplies bounded, typed proposals.
package worker

import (
	"context"
	"time"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ProtocolVersion      = "1.0"
	ContractVersion      = "1.0"
	EvidenceToolsFeature = "evidence_tools.v1"

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
	if err := validateHandshake(req); err != nil {
		return nil, err
	}
	if err := s.requireFeatures(req.GetRequestedFeatures()); err != nil {
		return nil, err
	}
	return s.handshakeResponse(), nil
}

func validateHandshake(req *runtimev1.HandshakeRequest) error {
	if req == nil || req.GetWorkerId() == "" || req.GetRuntimeInstanceId() == "" {
		return wireError(codes.InvalidArgument, "worker_id and runtime_instance_id are required")
	}
	if req.GetProtocolVersion() != ProtocolVersion {
		return wireErrorf(codes.FailedPrecondition, "unsupported protocol version %q", req.GetProtocolVersion())
	}
	if req.GetContractVersion() != ContractVersion {
		return wireErrorf(codes.FailedPrecondition, "unsupported contract version %q", req.GetContractVersion())
	}
	if !req.GetNonInteractive() {
		return wireError(codes.FailedPrecondition, "non_interactive worker handshake is required")
	}
	return nil
}

func (s *Server) requireFeatures(requestedFeatures []string) error {
	features := make(map[string]struct{}, len(s.SupportedFeatures))
	for _, feature := range s.SupportedFeatures {
		features[feature] = struct{}{}
	}
	for _, requested := range requestedFeatures {
		if _, ok := features[requested]; !ok {
			return wireErrorf(codes.FailedPrecondition, "unsupported required feature %q", requested)
		}
	}
	return nil
}

func (s *Server) handshakeResponse() *runtimev1.HandshakeResponse {
	maxRequest, maxEvent := s.limits()
	return &runtimev1.HandshakeResponse{
		ProtocolVersion:   ProtocolVersion,
		ContractVersion:   ContractVersion,
		WorkerName:        s.WorkerName,
		WorkerVersion:     s.WorkerVersion,
		SupportedFeatures: append([]string(nil), s.SupportedFeatures...),
		MaxRequestBytes:   maxRequest,
		MaxEventBytes:     maxEvent,
	}
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
	return &runtimev1.EpisodeEvent{
		EpisodeId:  req.GetEpisodeId(),
		Sequence:   1,
		OccurredAt: timestamppb.New(s.now()),
		AttemptId:  req.GetAttemptId(),
		Fence:      req.GetFence(),
		Payload: &runtimev1.EpisodeEvent_Started{Started: &runtimev1.EpisodeStarted{
			WorkerName: s.WorkerName, WorkerVersion: s.WorkerVersion,
		}},
	}
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
