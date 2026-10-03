package worker

import (
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func (s *Server) validateRequest(req *runtimev1.EpisodeRequest) error { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	if err := s.validateRequestProtocol(req); err != nil {
		return err
	}
	if err := validateRequestIdentity(req); err != nil {
		return err
	}
	if err := ValidateBudget(req.GetBudget()); err != nil {
		return wireErrorf(codes.InvalidArgument, "episode budget: %v", err)
	}
	return s.validateRequestContext(req)
}

// validateRequestIdentity requires the attempt identity, episode shape, and
// the digest-bound documents the worker reasons over.
func validateRequestIdentity(req *runtimev1.EpisodeRequest) error {
	for _, field := range []struct{ name, value string }{
		{"episode_id", req.GetEpisodeId()}, {"tenant_id", req.GetTenantId()},
		{"situation_id", req.GetSituationId()}, {"attempt_id", req.GetAttemptId()},
	} {
		if strings.TrimSpace(field.value) == "" {
			return wireErrorf(codes.InvalidArgument, "%s is required", field.name)
		}
	}
	return validateRequestShape(req)
}

func (s *Server) validateDeadline(req *runtimev1.EpisodeRequest) error {
	if req.GetDeadline() == nil {
		return nil
	}
	if !req.GetDeadline().IsValid() {
		return wireError(codes.InvalidArgument, "deadline is invalid")
	}
	if req.GetDeadline().AsTime().Before(s.now()) {
		return wireError(codes.DeadlineExceeded, "episode deadline has expired")
	}
	return nil
}

// validateEvidenceEndpoint requires the evidence endpoint and capability
// together, with the endpoint on a private Unix socket.
func validateEvidenceEndpoint(req *runtimev1.EpisodeRequest) error {
	if (req.GetEvidenceToolsEndpoint() == "") != (len(req.GetCapabilityToken()) == 0) {
		return wireError(codes.InvalidArgument, "evidence endpoint and capability token must be supplied together")
	}
	if req.GetEvidenceToolsEndpoint() != "" {
		if err := ValidateEvidenceSocketPath(req.GetEvidenceToolsEndpoint()); err != nil {
			return wireError(codes.PermissionDenied, "evidence endpoint must be a private Unix socket")
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
	eventBytes, err := v.validateEvent(event)
	if err != nil {
		return err
	}
	return v.sendEvent(stream, event, eventBytes)
}

func (v *streamValidator) checkSize(eventBytes uint64) error {
	if eventBytes > v.maxEventBytes {
		return wireError(codes.ResourceExhausted, "episode event exceeds size limit")
	}
	if v.eventCount >= v.maxEvents || v.streamBytes+eventBytes > v.maxStreamBytes {
		return wireError(codes.ResourceExhausted, "episode stream exceeds size limit")
	}
	return nil
}

// checkOrder requires the request's attempt identity, gapless sequencing, and
// nothing after the terminal.
func (v *streamValidator) checkOrder(event *runtimev1.EpisodeEvent) error {
	if v.terminal {
		return wireError(codes.FailedPrecondition, "event emitted after terminal")
	}
	if event.GetEpisodeId() != v.episodeID || event.GetAttemptId() != v.attemptID || event.GetFence() != v.fence {
		return wireError(codes.PermissionDenied, "episode event identity does not match request")
	}
	if event.GetSequence() == 0 || event.GetSequence() != v.nextSequence+1 {
		return wireErrorf(codes.FailedPrecondition, "episode event sequence %d is not %d", event.GetSequence(), v.nextSequence+1)
	}
	return nil
}

// checkPayload requires a timestamped payload, a Decision bound to this
// attempt with a SHA-256 digest, and a terminal with an explicit status.
func (v *streamValidator) checkPayload(event *runtimev1.EpisodeEvent) error {
	if event.GetOccurredAt() == nil || !event.GetOccurredAt().IsValid() || event.GetPayload() == nil {
		return wireError(codes.InvalidArgument, "episode event timestamp and payload are required")
	}
	if decision := event.GetDecision(); decision != nil {
		if decision.GetEpisodeId() != v.episodeID || decision.GetAttemptId() != v.attemptID || decision.GetFence() != v.fence || len(decision.GetDecisionJson()) == 0 || len(decision.GetDecisionSha256()) != 32 {
			return wireError(codes.PermissionDenied, "decision identity or digest is invalid")
		}
	}
	if terminal := event.GetTerminal(); terminal != nil && terminal.GetStatus() == runtimev1.TerminalStatus_TERMINAL_STATUS_UNSPECIFIED {
		return wireError(codes.InvalidArgument, "terminal status is required")
	}
	return nil
}

func (s *Server) validateRequestProtocol(req *runtimev1.EpisodeRequest) error {
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
	return nil
}

func (s *Server) validateRequestContext(req *runtimev1.EpisodeRequest) error {
	if _, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate()); err != nil {
		return wireErrorf(codes.InvalidArgument, "trace context: %v", err)
	}
	if err := s.validateDeadline(req); err != nil {
		return err
	}
	return validateEvidenceEndpoint(req)
}

func validateRequestShape(req *runtimev1.EpisodeRequest) error {
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
	return nil
}

func (v *streamValidator) validateEvent(event *runtimev1.EpisodeEvent) (uint64, error) {
	if event == nil {
		return 0, wireError(codes.InvalidArgument, "nil episode event")
	}
	eventBytes := uint64(proto.Size(event)) //nolint:gosec // protobuf Size is non-negative and bounded by the configured event limit.
	if err := v.checkSize(eventBytes); err != nil {
		return 0, err
	}
	if err := v.checkOrder(event); err != nil {
		return 0, err
	}
	if err := v.checkPayload(event); err != nil {
		return 0, err
	}
	return eventBytes, nil
}

func (v *streamValidator) sendEvent(stream runtimev1.EpisodeWorker_ExecuteServer, event *runtimev1.EpisodeEvent, eventBytes uint64) error {
	if event.GetTerminal() != nil {
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
