package workerfake

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestValidateRequestCodesEveryDefect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runtimev1.EpisodeRequest)
		limits Limits
		want   codes.Code
	}{
		{"valid request", func(*runtimev1.EpisodeRequest) {}, Limits{}, codes.OK},
		{"request over the size limit", func(*runtimev1.EpisodeRequest) {}, Limits{MaxRequestBytes: 1}, codes.ResourceExhausted},
		{"another protocol version", func(r *runtimev1.EpisodeRequest) { r.ProtocolVersion = "9" }, Limits{}, codes.FailedPrecondition},
		{"blank episode", func(r *runtimev1.EpisodeRequest) { r.EpisodeId = " " }, Limits{}, codes.InvalidArgument},
		{"missing tenant", func(r *runtimev1.EpisodeRequest) { r.TenantId = "" }, Limits{}, codes.InvalidArgument},
		{"missing situation", func(r *runtimev1.EpisodeRequest) { r.SituationId = "" }, Limits{}, codes.InvalidArgument},
		{"missing attempt", func(r *runtimev1.EpisodeRequest) { r.AttemptId = "" }, Limits{}, codes.InvalidArgument},
		{"zero fence", func(r *runtimev1.EpisodeRequest) { r.Fence = 0 }, Limits{}, codes.InvalidArgument},
		{"zero situation version", func(r *runtimev1.EpisodeRequest) { r.SituationVersion = 0 }, Limits{}, codes.InvalidArgument},
		{"unspecified kind", func(r *runtimev1.EpisodeRequest) { r.Kind = runtimev1.EpisodeKind_EPISODE_KIND_UNSPECIFIED }, Limits{}, codes.InvalidArgument},
		{"unspecified lane", func(r *runtimev1.EpisodeRequest) { r.Lane = runtimev1.EpisodeLane_EPISODE_LANE_UNSPECIFIED }, Limits{}, codes.InvalidArgument},
		{"unspecified risk ceiling", func(r *runtimev1.EpisodeRequest) { r.RiskCeiling = runtimev1.RiskClass_RISK_CLASS_UNSPECIFIED }, Limits{}, codes.InvalidArgument},
		{"short snapshot digest", func(r *runtimev1.EpisodeRequest) { r.SnapshotSha256 = nil }, Limits{}, codes.InvalidArgument},
		{"short spec digest", func(r *runtimev1.EpisodeRequest) { r.SpecSha256 = make([]byte, 31) }, Limits{}, codes.InvalidArgument},
		{"missing snapshot", func(r *runtimev1.EpisodeRequest) { r.SnapshotJson = nil }, Limits{}, codes.InvalidArgument},
		{"missing decision schema", func(r *runtimev1.EpisodeRequest) { r.DecisionSchemaJson = nil }, Limits{}, codes.InvalidArgument},
		{"missing tool catalog", func(r *runtimev1.EpisodeRequest) { r.ToolCatalogJson = nil }, Limits{}, codes.InvalidArgument},
		{"missing budget", func(r *runtimev1.EpisodeRequest) { r.Budget = nil }, Limits{}, codes.InvalidArgument},
		{"malformed trace context", func(r *runtimev1.EpisodeRequest) { r.Traceparent = "not-a-trace" }, Limits{}, codes.InvalidArgument},
		{"expired deadline", func(r *runtimev1.EpisodeRequest) { r.Deadline = timestamppb.New(fixedNow.Add(-time.Second)) }, Limits{}, codes.DeadlineExceeded},
		{"future deadline", func(r *runtimev1.EpisodeRequest) { r.Deadline = timestamppb.New(fixedNow.Add(time.Second)) }, Limits{}, codes.OK},
		{"invalid deadline", func(r *runtimev1.EpisodeRequest) { r.Deadline = &timestamppb.Timestamp{Seconds: 1 << 60} }, Limits{}, codes.InvalidArgument},
		{"evidence endpoint without a capability", func(r *runtimev1.EpisodeRequest) { r.EvidenceToolsEndpoint = "/tmp/e.sock" }, Limits{}, codes.InvalidArgument},
		{"capability without an evidence endpoint", func(r *runtimev1.EpisodeRequest) { r.CapabilityToken = []byte("token") }, Limits{}, codes.InvalidArgument},
		{"evidence endpoint that is not a private socket path", func(r *runtimev1.EpisodeRequest) {
			r.EvidenceToolsEndpoint, r.CapabilityToken = "tcp://evidence:80", []byte("token")
		}, Limits{}, codes.PermissionDenied},
		{"evidence endpoint with its capability", func(r *runtimev1.EpisodeRequest) {
			r.EvidenceToolsEndpoint, r.CapabilityToken = "/tmp/e.sock", []byte("token")
		}, Limits{}, codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validRequest()
			tt.mutate(req)
			if got := status.Code(ValidateRequest(req, tt.limits.Resolved(), fixedNow)); got != tt.want {
				t.Fatalf("ValidateRequest() code = %v, want %v", got, tt.want)
			}
		})
	}
	if got := status.Code(ValidateRequest(nil, Limits{}.Resolved(), fixedNow)); got != codes.InvalidArgument {
		t.Fatalf("ValidateRequest(nil) code = %v, want InvalidArgument", got)
	}
}

func TestValidateRequestChecksProtocolBeforeIdentityBeforeBudget(t *testing.T) {
	t.Parallel()
	req := validRequest()
	req.ProtocolVersion, req.EpisodeId, req.Budget = "9", "", nil
	if got := status.Code(ValidateRequest(req, Limits{}.Resolved(), fixedNow)); got != codes.FailedPrecondition {
		t.Fatalf("a request with several defects reported %v, want the protocol version (FailedPrecondition) first", got)
	}
	req.ProtocolVersion = validRequest().ProtocolVersion
	if got := status.Code(ValidateRequest(req, Limits{}.Resolved(), fixedNow)); got != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument for the identity", got)
	}
}
