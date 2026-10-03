package remote

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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
			stream := newWorkerStream(request, &runtimev1.EpisodeBudget{MaxModelCalls: 1}, test.limit)
			stream.sawStarted, stream.nextSequence, stream.usage.modelCalls = true, 2, 1
			event := &runtimev1.EpisodeEvent{EpisodeId: request.EpisodeID, AttemptId: request.AttemptID, Fence: 7, Sequence: 2, OccurredAt: timestamppb.New(time.Unix(10, 0)), Payload: &runtimev1.EpisodeEvent_ModelStarted{ModelStarted: &runtimev1.ModelStarted{}}}
			if test.mismatch {
				event.AttemptId = "foreign-attempt"
			}
			err := stream.accept(event)
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
	if _, err := episodeRequest(req); err == nil || err.Error() != "snapshot is required" {
		t.Fatalf("mapping error = %v", err)
	}
}

func TestTerminalUsageFailurePrecedesOutcomeAssembly(t *testing.T) {
	t.Parallel()
	stream := newWorkerStream(validWorkerRequest(), &runtimev1.EpisodeBudget{MaxInputTokens: 2}, 0)
	stream.sawStarted, stream.sawBudget = true, true
	stream.terminal = &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, Usage: &runtimev1.Usage{InputTokens: 3}}
	_, err := stream.outcome()
	var exceeded *episodes.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Metric != "input_tokens" {
		t.Fatalf("outcome error = %v, want input_tokens budget error before missing decision", err)
	}
}
