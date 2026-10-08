package workerfake

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	"google.golang.org/grpc/codes"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// ValidateHandshake accepts only the current protocol and contract versions
// from a non-interactive runtime that names itself and its instance.
func ValidateHandshake(req *runtimev1.HandshakeRequest) error {
	if req == nil || req.GetWorkerId() == "" || req.GetRuntimeInstanceId() == "" {
		return WireError(codes.InvalidArgument, "worker_id and runtime_instance_id are required")
	}
	if req.GetProtocolVersion() != worker.ProtocolVersion {
		return WireErrorf(codes.FailedPrecondition, "unsupported protocol version %q", req.GetProtocolVersion())
	}
	if req.GetContractVersion() != worker.ContractVersion {
		return WireErrorf(codes.FailedPrecondition, "unsupported contract version %q", req.GetContractVersion())
	}
	if !req.GetNonInteractive() {
		return WireError(codes.FailedPrecondition, "non_interactive worker handshake is required")
	}
	return nil
}

// RequireFeatures rejects a requested feature the worker does not advertise.
func RequireFeatures(supported, requested []string) error {
	features := make(map[string]struct{}, len(supported))
	for _, feature := range supported {
		features[feature] = struct{}{}
	}
	for _, feature := range requested {
		if _, ok := features[feature]; !ok {
			return WireErrorf(codes.FailedPrecondition, "unsupported required feature %q", feature)
		}
	}
	return nil
}

// NewHandshakeResponse advertises the worker's identity, features and limits.
func NewHandshakeResponse(name, version string, features []string, limits Limits) *runtimev1.HandshakeResponse {
	return &runtimev1.HandshakeResponse{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerName: name, WorkerVersion: version,
		SupportedFeatures: append([]string(nil), features...),
		MaxRequestBytes:   limits.MaxRequestBytes, MaxEventBytes: limits.MaxEventBytes,
	}
}
