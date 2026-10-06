package domain

import (
	"google.golang.org/protobuf/proto"

	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestStreamAdmissionPreservesErrorPrecedence(t *testing.T) {
	t.Parallel()
	request := validWorkerRequest()
	for _, test := range []struct {
		name     string
		limit    uint64
		mismatch bool
		want     string
	}{
		{name: "size before identity", limit: 1, mismatch: true, want: "worker event exceeds negotiated size limit"},
		{name: "identity before budget", mismatch: true, want: "worker event identity or sequence mismatch"},
		{name: "budget before payload", want: "model_calls"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := NewStream(request, &runtimev1.EpisodeBudget{MaxModelCalls: 1}, test.limit)
			stream.sawStarted, stream.nextSequence, stream.usage.modelCalls = true, 2, 1
			event := &runtimev1.EpisodeEvent{EpisodeId: request.EpisodeID, AttemptId: request.AttemptID, Fence: 7, Sequence: 2, OccurredAt: timestamppb.New(time.Unix(10, 0)), Payload: &runtimev1.EpisodeEvent_ModelStarted{ModelStarted: &runtimev1.ModelStarted{}}}
			if test.mismatch {
				event.AttemptId = "foreign-attempt"
			}
			err := stream.Accept(event)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("accept error = %v, want %q", err, test.want)
			}
			if stream.nextSequence != 2 {
				t.Fatalf("failed admission advanced sequence to %d", stream.nextSequence)
			}
		})
	}
}

func TestRequestMappingPreservesValidationPrecedence(t *testing.T) {
	t.Parallel()
	req := validWorkerRequest()
	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatal(err)
	}
	delete(payload, "snapshot")
	payload["kind"] = "unsupported"
	payload["budget"] = map[string]any{"wall_time": "broken"}
	req.RequestJSON, _ = json.Marshal(payload)
	req.SnapshotSHA256 = "broken"
	if _, err := WireRequest(req); err == nil || err.Error() != "snapshot is required" {
		t.Fatalf("mapping error = %v", err)
	}
}

func TestTerminalUsageFailurePrecedesOutcomeAssembly(t *testing.T) {
	t.Parallel()
	stream := NewStream(validWorkerRequest(), &runtimev1.EpisodeBudget{MaxInputTokens: 2}, 0)
	stream.sawStarted, stream.sawBudget = true, true
	stream.terminal = &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, Usage: &runtimev1.Usage{InputTokens: 3}}
	_, err := stream.Outcome()
	var exceeded *episodes.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Metric != "input_tokens" {
		t.Fatalf("outcome error = %v, want input_tokens budget error before missing decision", err)
	}
}

func TestProfileHandshakeValidation(t *testing.T) {
	t.Parallel()
	profile := Profile{Name: "worker-1", RuntimeInstance: "rt", RequestedFeatures: []string{worker.EvidenceToolsFeature}}
	request := profile.HandshakeRequest()
	if request.GetWorkerId() != "worker-1" || !request.GetNonInteractive() || request.GetProtocolVersion() != worker.ProtocolVersion {
		t.Fatalf("handshake request = %v", request)
	}
	ok := &runtimev1.HandshakeResponse{ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion, WorkerName: "worker-1", SupportedFeatures: []string{worker.EvidenceToolsFeature}}
	if err := profile.ValidateHandshake(ok, &runtimev1.EpisodeRequest{}); err != nil {
		t.Fatalf("valid handshake: %v", err)
	}
	for name, mutate := range map[string]func(*runtimev1.HandshakeResponse){
		"version":  func(r *runtimev1.HandshakeResponse) { r.ProtocolVersion = "0.9" },
		"identity": func(r *runtimev1.HandshakeResponse) { r.WorkerName = "other" },
		"feature":  func(r *runtimev1.HandshakeResponse) { r.SupportedFeatures = nil },
		"size":     func(r *runtimev1.HandshakeResponse) { r.MaxRequestBytes = 1 },
	} {
		response := proto.Clone(ok).(*runtimev1.HandshakeResponse)
		mutate(response)
		if err := profile.ValidateHandshake(response, &runtimev1.EpisodeRequest{Kind: runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER, EpisodeId: "a-long-enough-episode-id"}); err == nil {
			t.Errorf("%s mismatch accepted", name)
		}
	}
}

func TestProfileEvidenceConfigFailsClosed(t *testing.T) {
	t.Parallel()
	if err := (Profile{}).ValidateEvidenceConfig("", false); err != nil {
		t.Fatalf("no endpoint must pass: %v", err)
	}
	if err := (Profile{}).ValidateEvidenceConfig("relative.sock", true); err == nil {
		t.Fatal("relative evidence endpoint accepted")
	}
	profile := Profile{RequestedFeatures: []string{worker.EvidenceToolsFeature}}
	if err := profile.ValidateEvidenceConfig("/tmp/evidence.sock", false); err == nil {
		t.Fatal("missing capability factory accepted")
	}
	if err := (Profile{}).ValidateEvidenceConfig("/tmp/evidence.sock", true); err == nil {
		t.Fatal("missing negotiated feature accepted")
	}
}

func TestStreamOutcomeRefusesIncompleteStreams(t *testing.T) {
	t.Parallel()
	stream := NewStream(validWorkerRequest(), &runtimev1.EpisodeBudget{}, 0)
	if _, err := stream.Outcome(); err == nil {
		t.Fatal("empty stream produced an outcome")
	}
	if err := stream.Accept(nil); err == nil {
		t.Fatal("nil event accepted")
	}
	if err := stream.Accept(&runtimev1.EpisodeEvent{}); err == nil {
		t.Fatal("event with another identity accepted")
	}
}

func TestBudgetUsageStopsAtModelCallCeiling(t *testing.T) {
	t.Parallel()
	limit := &runtimev1.EpisodeBudget{MaxModelCalls: 1}
	started := &runtimev1.EpisodeEvent{Payload: &runtimev1.EpisodeEvent_ModelStarted{ModelStarted: &runtimev1.ModelStarted{}}}
	var usage budgetUsage
	if err := usage.observe(limit, started); err != nil {
		t.Fatalf("first model call: %v", err)
	}
	if err := usage.observe(limit, started); err == nil {
		t.Fatal("second model call exceeded the ceiling unnoticed")
	}
	if err := usage.observe(nil, started); err != nil {
		t.Fatalf("no limit must pass: %v", err)
	}
}

func TestStreamBindsTerminalStatusToOutcome(t *testing.T) {
	t.Parallel()
	req := validWorkerRequest()
	event := func(sequence uint64, payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
		e := &runtimev1.EpisodeEvent{EpisodeId: req.EpisodeID, AttemptId: req.AttemptID, Fence: uint64(req.Fence), Sequence: sequence, OccurredAt: timestamppb.Now()} //nolint:gosec // Fence is non-negative in the fixture.
		payload(e)
		return e
	}
	for status, want := range map[runtimev1.TerminalStatus]string{
		runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED:  "declined",
		runtimev1.TerminalStatus_TERMINAL_STATUS_TIMED_OUT: "timed_out",
		runtimev1.TerminalStatus_TERMINAL_STATUS_FAILED:    "failed",
	} {
		stream := NewStream(req, &runtimev1.EpisodeBudget{}, 0)
		if err := stream.Accept(event(1, func(e *runtimev1.EpisodeEvent) {
			e.Payload = &runtimev1.EpisodeEvent_Started{Started: &runtimev1.EpisodeStarted{}}
		})); err != nil {
			t.Fatal(err)
		}
		if err := stream.Accept(event(2, func(e *runtimev1.EpisodeEvent) {
			e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: status, ReasonCode: "r"}}
		})); err != nil {
			t.Fatal(err)
		}
		outcome, err := stream.Outcome()
		if err != nil || outcome.Status != want {
			t.Fatalf("status %v: outcome %+v err %v, want %s", status, outcome, err, want)
		}
	}
}
