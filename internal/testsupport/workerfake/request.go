package workerfake

import (
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// ValidateRequest checks one episode request against the protocol: size,
// version, identity, shape, budget, trace context, deadline and evidence
// endpoint. The check order is part of the contract.
func ValidateRequest(req *runtimev1.EpisodeRequest, limits Limits, now time.Time) error {
	if err := validateRequestProtocol(req, limits); err != nil {
		return err
	}
	if err := validateRequestIdentity(req); err != nil {
		return err
	}
	if err := worker.ValidateBudget(req.GetBudget()); err != nil {
		return WireErrorf(codes.InvalidArgument, "episode budget: %v", err)
	}
	return validateRequestContext(req, now)
}

func validateRequestProtocol(req *runtimev1.EpisodeRequest, limits Limits) error {
	if req == nil {
		return WireError(codes.InvalidArgument, "episode request is required")
	}
	if uint64(proto.Size(req)) > limits.MaxRequestBytes { //nolint:gosec // protobuf Size is non-negative and bounded by the configured request limit.
		return WireError(codes.ResourceExhausted, "episode request exceeds size limit")
	}
	if strings.TrimSpace(req.GetProtocolVersion()) != strings.TrimSpace(worker.ProtocolVersion) {
		return WireErrorf(codes.FailedPrecondition, "unsupported protocol version %q", req.GetProtocolVersion())
	}
	return nil
}

// validateRequestIdentity requires the attempt identity, episode shape, and
// the digest-bound documents the worker reasons over.
func validateRequestIdentity(req *runtimev1.EpisodeRequest) error {
	for _, field := range []struct{ name, value string }{
		{"episode_id", req.GetEpisodeId()}, {"tenant_id", req.GetTenantId()},
		{"situation_id", req.GetSituationId()}, {"attempt_id", req.GetAttemptId()},
	} {
		if strings.TrimSpace(field.value) == "" {
			return WireErrorf(codes.InvalidArgument, "%s is required", field.name)
		}
	}
	return validateRequestShape(req)
}

func validateRequestShape(req *runtimev1.EpisodeRequest) error {
	if req.GetFence() == 0 || req.GetSituationVersion() == 0 {
		return WireError(codes.InvalidArgument, "situation_version and fence must be positive")
	}
	if req.GetKind() == runtimev1.EpisodeKind_EPISODE_KIND_UNSPECIFIED || req.GetLane() == runtimev1.EpisodeLane_EPISODE_LANE_UNSPECIFIED || req.GetRiskCeiling() == runtimev1.RiskClass_RISK_CLASS_UNSPECIFIED {
		return WireError(codes.InvalidArgument, "kind, lane, and risk_ceiling are required")
	}
	if len(req.GetSnapshotSha256()) != 32 || len(req.GetSpecSha256()) != 32 {
		return WireError(codes.InvalidArgument, "snapshot_sha256 and spec_sha256 must be 32 bytes")
	}
	if len(req.GetSnapshotJson()) == 0 || len(req.GetDecisionSchemaJson()) == 0 || len(req.GetToolCatalogJson()) == 0 {
		return WireError(codes.InvalidArgument, "snapshot, decision schema, and tool catalog are required")
	}
	return nil
}

func validateRequestContext(req *runtimev1.EpisodeRequest, now time.Time) error {
	if _, err := contractsv1.ParseTraceContext(req.GetTraceparent(), req.GetTracestate()); err != nil {
		return WireErrorf(codes.InvalidArgument, "trace context: %v", err)
	}
	if err := validateDeadline(req, now); err != nil {
		return err
	}
	return validateEvidenceEndpoint(req)
}

func validateDeadline(req *runtimev1.EpisodeRequest, now time.Time) error {
	if req.GetDeadline() == nil {
		return nil
	}
	if !req.GetDeadline().IsValid() {
		return WireError(codes.InvalidArgument, "deadline is invalid")
	}
	if req.GetDeadline().AsTime().Before(now) {
		return WireError(codes.DeadlineExceeded, "episode deadline has expired")
	}
	return nil
}

// validateEvidenceEndpoint requires the evidence endpoint and capability
// together, with the endpoint on a private Unix socket.
func validateEvidenceEndpoint(req *runtimev1.EpisodeRequest) error {
	if (req.GetEvidenceToolsEndpoint() == "") != (len(req.GetCapabilityToken()) == 0) {
		return WireError(codes.InvalidArgument, "evidence endpoint and capability token must be supplied together")
	}
	if req.GetEvidenceToolsEndpoint() != "" {
		if err := worker.ValidateEvidenceSocketPath(req.GetEvidenceToolsEndpoint()); err != nil {
			return WireError(codes.PermissionDenied, "evidence endpoint must be a private Unix socket")
		}
	}
	return nil
}
