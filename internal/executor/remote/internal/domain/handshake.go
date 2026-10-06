package domain

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// Profile is the runtime's expectation of one worker: its identity and the
// features it must negotiate.
type Profile struct {
	Name              string
	RuntimeInstance   string
	RequestedFeatures []string
}

// HandshakeRequest is the current-version, non-interactive handshake.
func (p Profile) HandshakeRequest() *runtimev1.HandshakeRequest {
	return &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: p.Name, RuntimeInstanceId: p.RuntimeInstance, NonInteractive: true,
		RequestedFeatures: slices.Clone(p.RequestedFeatures),
	}
}

// ValidateHandshake requires the exact protocol and contract versions, the
// expected worker identity, every requested feature, and a request within the
// worker's size limit.
func (p Profile) ValidateHandshake(handshake *runtimev1.HandshakeResponse, wireRequest *runtimev1.EpisodeRequest) error {
	if handshake.GetProtocolVersion() != worker.ProtocolVersion || handshake.GetContractVersion() != worker.ContractVersion {
		return fmt.Errorf("worker handshake returned unsupported versions")
	}
	if p.Name != "" && handshake.GetWorkerName() != p.Name {
		return fmt.Errorf("worker handshake identity mismatch")
	}
	return p.validateNegotiatedLimits(handshake, wireRequest)
}

func (p Profile) validateNegotiatedLimits(handshake *runtimev1.HandshakeResponse, wireRequest *runtimev1.EpisodeRequest) error {
	for _, requested := range p.RequestedFeatures {
		if !slices.Contains(handshake.GetSupportedFeatures(), requested) {
			return fmt.Errorf("worker did not negotiate requested feature %q", requested)
		}
	}
	if handshake.GetMaxRequestBytes() > 0 && uint64(proto.Size(wireRequest)) > handshake.GetMaxRequestBytes() { //nolint:gosec // protobuf Size is non-negative and bounded by the negotiated request limit.
		return fmt.Errorf("worker request exceeds negotiated size limit")
	}
	return nil
}

// ValidateEvidenceConfig requires, when an evidence endpoint is configured, a
// valid socket path, the negotiated evidence feature and a capability factory.
func (p Profile) ValidateEvidenceConfig(endpoint string, hasCapabilityFactory bool) error {
	if endpoint == "" {
		return nil
	}
	if err := worker.ValidateEvidenceSocketPath(endpoint); err != nil {
		return fmt.Errorf("evidence endpoint: %w", err)
	}
	if !slices.Contains(p.RequestedFeatures, worker.EvidenceToolsFeature) {
		return fmt.Errorf("evidence tools require negotiated feature %q", worker.EvidenceToolsFeature)
	}
	if !hasCapabilityFactory {
		return fmt.Errorf("evidence capability factory is not configured")
	}
	return nil
}
