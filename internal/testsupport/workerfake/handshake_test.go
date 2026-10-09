package workerfake

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestValidateHandshakeAcceptsOnlyTheCurrentNonInteractiveRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runtimev1.HandshakeRequest)
		want   codes.Code
	}{
		{"current runtime", func(*runtimev1.HandshakeRequest) {}, codes.OK},
		{"missing worker id", func(r *runtimev1.HandshakeRequest) { r.WorkerId = "" }, codes.InvalidArgument},
		{"missing runtime instance", func(r *runtimev1.HandshakeRequest) { r.RuntimeInstanceId = "" }, codes.InvalidArgument},
		{"another protocol version", func(r *runtimev1.HandshakeRequest) { r.ProtocolVersion = "9" }, codes.FailedPrecondition},
		{"another minor protocol version", func(r *runtimev1.HandshakeRequest) { r.ProtocolVersion = "1.1" }, codes.FailedPrecondition},
		{"another contract version", func(r *runtimev1.HandshakeRequest) { r.ContractVersion = "9" }, codes.FailedPrecondition},
		{"interactive runtime", func(r *runtimev1.HandshakeRequest) { r.NonInteractive = false }, codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := validHandshake()
			tt.mutate(request)
			if got := status.Code(ValidateHandshake(request)); got != tt.want {
				t.Fatalf("ValidateHandshake() code = %v, want %v", got, tt.want)
			}
		})
	}
	if got := status.Code(ValidateHandshake(nil)); got != codes.InvalidArgument {
		t.Fatalf("ValidateHandshake(nil) code = %v, want InvalidArgument", got)
	}
}

func TestRequireFeaturesRejectsAFeatureTheWorkerDoesNotAdvertise(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		supported, request []string
		want               codes.Code
	}{
		{"nothing requested", []string{"a"}, nil, codes.OK},
		{"every requested feature offered", []string{"a", "b"}, []string{"b", "a"}, codes.OK},
		{"one feature missing", []string{"a"}, []string{"a", "b"}, codes.FailedPrecondition},
		{"nothing offered", nil, []string{"a"}, codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := status.Code(RequireFeatures(tt.supported, tt.request)); got != tt.want {
				t.Fatalf("RequireFeatures() code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandshakeResponseAdvertisesIdentityFeaturesAndLimits(t *testing.T) {
	t.Parallel()
	features := []string{"a"}
	response := NewHandshakeResponse("n", "v", features, Limits{MaxRequestBytes: 7}.Resolved())
	features[0] = "mutated"
	if response.GetWorkerName() != "n" || response.GetWorkerVersion() != "v" || response.GetProtocolVersion() != worker.ProtocolVersion ||
		response.GetContractVersion() != worker.ContractVersion || response.GetMaxRequestBytes() != 7 || response.GetMaxEventBytes() != defaultMaxEventBytes ||
		len(response.GetSupportedFeatures()) != 1 || response.GetSupportedFeatures()[0] != "a" {
		t.Fatalf("response = %v", response)
	}
}
