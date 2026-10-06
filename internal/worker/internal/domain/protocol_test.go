package domain

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestLimitsResolveZeroBoundsToDefaults(t *testing.T) {
	t.Parallel()
	got := Limits{MaxEvents: 7}.Resolved()
	if got.MaxEvents != 7 || got.MaxRequestBytes != DefaultMaxRequestBytes || got.MaxEventBytes != DefaultMaxEventBytes || got.MaxStreamBytes != DefaultMaxStreamBytes {
		t.Fatalf("limits = %+v", got)
	}
}

func TestSocketPathRuleAcceptsOnlyCleanAbsoluteUnixPaths(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "relative.sock", "unix:///tmp/x.sock", "/tmp/../x.sock", "/tmp/x\x00.sock"} {
		if ValidateEvidenceSocketPath(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := ValidateEvidenceSocketPath("/tmp/evidence.sock"); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetRequiresAPositiveWallTime(t *testing.T) {
	t.Parallel()
	if ValidateBudget(nil) == nil || ValidateBudget(&runtimev1.EpisodeBudget{}) == nil {
		t.Fatal("budget without wall time accepted")
	}
	if ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(0)}) == nil {
		t.Fatal("zero wall time accepted")
	}
	if err := ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)}); err != nil {
		t.Fatal(err)
	}
}

func TestHandshakeChecksVersionsIdentityAndFeatures(t *testing.T) {
	t.Parallel()
	ok := &runtimev1.HandshakeRequest{ProtocolVersion: ProtocolVersion, ContractVersion: ContractVersion, WorkerId: "w", RuntimeInstanceId: "r", NonInteractive: true}
	if err := ValidateHandshake(ok); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*runtimev1.HandshakeRequest){
		"id":      func(r *runtimev1.HandshakeRequest) { r.WorkerId = "" },
		"proto":   func(r *runtimev1.HandshakeRequest) { r.ProtocolVersion = "9" },
		"contrac": func(r *runtimev1.HandshakeRequest) { r.ContractVersion = "9" },
		"interac": func(r *runtimev1.HandshakeRequest) { r.NonInteractive = false },
	} {
		bad := &runtimev1.HandshakeRequest{ProtocolVersion: ok.ProtocolVersion, ContractVersion: ok.ContractVersion, WorkerId: "w", RuntimeInstanceId: "r", NonInteractive: true}
		mutate(bad)
		if ValidateHandshake(bad) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if ValidateHandshake(nil) == nil {
		t.Fatal("nil handshake accepted")
	}
	if err := RequireFeatures([]string{"a"}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if status.Code(RequireFeatures([]string{"a"}, []string{"b"})) != codes.FailedPrecondition {
		t.Fatal("missing feature must fail the precondition")
	}
	if response := NewHandshakeResponse("n", "v", []string{"a"}, Limits{}.Resolved()); response.GetWorkerName() != "n" || response.GetMaxEventBytes() != DefaultMaxEventBytes {
		t.Fatalf("response = %v", response)
	}
}

func validRequest() *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{
		ProtocolVersion: ProtocolVersion, EpisodeId: "e", TenantId: "t", SituationId: "s", AttemptId: "a",
		Fence: 1, SituationVersion: 1,
		Kind: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE, Lane: runtimev1.EpisodeLane_EPISODE_LANE_DEEP, RiskCeiling: runtimev1.RiskClass_RISK_CLASS_R1,
		SnapshotSha256: make([]byte, 32), SpecSha256: make([]byte, 32),
		SnapshotJson: []byte("{}"), DecisionSchemaJson: []byte("{}"), ToolCatalogJson: []byte("[]"),
		Budget: &runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)},
	}
}

func TestRequestValidationCoversSizeVersionShapeAndDeadline(t *testing.T) {
	t.Parallel()
	limits := Limits{}.Resolved()
	now := time.Unix(100, 0)
	if err := ValidateRequest(validRequest(), limits, now); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	for name, mutate := range map[string]func(*runtimev1.EpisodeRequest){
		"version":  func(r *runtimev1.EpisodeRequest) { r.ProtocolVersion = "9" },
		"identity": func(r *runtimev1.EpisodeRequest) { r.EpisodeId = " " },
		"shape":    func(r *runtimev1.EpisodeRequest) { r.Fence = 0 },
		"digests":  func(r *runtimev1.EpisodeRequest) { r.SnapshotSha256 = nil },
		"budget":   func(r *runtimev1.EpisodeRequest) { r.Budget = nil },
		"deadline": func(r *runtimev1.EpisodeRequest) { r.Deadline = timestamppb.New(time.Unix(1, 0)) },
		"evidence": func(r *runtimev1.EpisodeRequest) { r.EvidenceToolsEndpoint = "/tmp/e.sock" },
	} {
		req := validRequest()
		mutate(req)
		if ValidateRequest(req, limits, now) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if ValidateRequest(nil, limits, now) == nil || ValidateRequest(validRequest(), Limits{MaxRequestBytes: 1}, now) == nil {
		t.Fatal("nil or oversized request accepted")
	}
}

func TestStreamValidatorEnforcesSequenceIdentityAndSingleTerminal(t *testing.T) {
	t.Parallel()
	req := validRequest()
	validator := NewStreamValidator(req, Limits{}.Resolved())
	started := StartedEvent(req, "w", "v", time.Unix(1, 0))
	size, err := validator.Check(started)
	if err != nil {
		t.Fatalf("started: %v", err)
	}
	validator.Record(started, size)
	if _, err := validator.Check(started); err == nil {
		t.Fatal("repeated sequence accepted")
	}
	terminal := &runtimev1.EpisodeEvent{EpisodeId: "e", AttemptId: "a", Fence: 1, Sequence: 2, OccurredAt: timestamppb.Now(),
		Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED}}}
	size, err = validator.Check(terminal)
	if err != nil {
		t.Fatalf("terminal: %v", err)
	}
	validator.Record(terminal, size)
	if !validator.Terminated() {
		t.Fatal("terminal not recorded")
	}
	if _, err := validator.Check(terminal); err == nil {
		t.Fatal("event after terminal accepted")
	}
	if _, err := validator.Check(nil); err == nil {
		t.Fatal("nil event accepted")
	}
}
