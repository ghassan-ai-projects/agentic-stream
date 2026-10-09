package domain

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func evidenceProfile() Profile {
	return Profile{Name: "worker-1", RuntimeInstance: "rt", RequestedFeatures: []string{worker.EvidenceToolsFeature}}
}

func acceptableHandshake() *runtimev1.HandshakeResponse {
	return &runtimev1.HandshakeResponse{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerName: "worker-1", SupportedFeatures: []string{worker.EvidenceToolsFeature},
	}
}

func TestHandshakeRequestIsCurrentVersionAndNonInteractive(t *testing.T) {
	t.Parallel()
	profile := evidenceProfile()
	request := profile.HandshakeRequest()
	profile.RequestedFeatures[0] = "mutated"
	if request.GetWorkerId() != "worker-1" || request.GetRuntimeInstanceId() != "rt" || !request.GetNonInteractive() ||
		request.GetProtocolVersion() != worker.ProtocolVersion || request.GetContractVersion() != worker.ContractVersion {
		t.Fatalf("handshake request = %v", request)
	}
	if got := request.GetRequestedFeatures(); len(got) != 1 || got[0] != worker.EvidenceToolsFeature {
		t.Fatalf("requested features = %v, want an independent copy of the profile's", got)
	}
}

func TestHandshakeValidationRefusesWhatTheRuntimeDidNotAsk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runtimev1.HandshakeResponse)
		want   string
	}{
		{"protocol version", func(r *runtimev1.HandshakeResponse) { r.ProtocolVersion = "0.9" }, "unsupported versions"},
		{"contract version", func(r *runtimev1.HandshakeResponse) { r.ContractVersion = "0.9" }, "unsupported versions"},
		{"identity", func(r *runtimev1.HandshakeResponse) { r.WorkerName = "other" }, "identity mismatch"},
		{"requested feature", func(r *runtimev1.HandshakeResponse) { r.SupportedFeatures = nil }, `requested feature "evidence_tools.v1"`},
		{"request size limit", func(r *runtimev1.HandshakeResponse) { r.MaxRequestBytes = 1 }, "exceeds negotiated size limit"},
	}
	wire := &runtimev1.EpisodeRequest{EpisodeId: "a-long-enough-episode-id"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			response := proto.Clone(acceptableHandshake()).(*runtimev1.HandshakeResponse)
			tt.mutate(response)
			if err := evidenceProfile().ValidateHandshake(response, wire); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateHandshake() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestHandshakeValidationAcceptsTheNegotiatedWorker(t *testing.T) {
	t.Parallel()
	wire := &runtimev1.EpisodeRequest{EpisodeId: "e"}
	limited := acceptableHandshake()
	limited.MaxRequestBytes = uint64(proto.Size(wire)) //nolint:gosec // protobuf Size is non-negative.
	for name, response := range map[string]*runtimev1.HandshakeResponse{"unlimited": acceptableHandshake(), "request exactly at the limit": limited} {
		if err := evidenceProfile().ValidateHandshake(response, wire); err != nil {
			t.Errorf("%s: ValidateHandshake() = %v", name, err)
		}
	}
	anonymous := Profile{}
	if err := anonymous.ValidateHandshake(acceptableHandshake(), wire); err != nil {
		t.Errorf("a profile without a name must accept any worker name: %v", err)
	}
}

func TestEvidenceConfigurationFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		profile       Profile
		endpoint      string
		hasCapability bool
		want          string
	}{
		{"no endpoint means no evidence tools", Profile{}, "", false, ""},
		{"endpoint with feature and capability", evidenceProfile(), "/run/evidence.sock", true, ""},
		{"relative endpoint", evidenceProfile(), "relative.sock", true, "evidence endpoint"},
		{"unclean endpoint", evidenceProfile(), "/run/../evidence.sock", true, "evidence endpoint"},
		{"feature not requested", Profile{}, "/run/evidence.sock", true, "require negotiated feature"},
		{"no capability factory", evidenceProfile(), "/run/evidence.sock", false, "capability factory is not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.profile.ValidateEvidenceConfig(tt.endpoint, tt.hasCapability)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("ValidateEvidenceConfig() = %v, want %q", err, tt.want)
			}
		})
	}
}
