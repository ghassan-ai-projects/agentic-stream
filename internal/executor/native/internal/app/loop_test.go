package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestExecuteFeedsToolObservationsToTheNextModelCall(t *testing.T) {
	t.Parallel()
	var seenArguments json.RawMessage
	tool := toolFunc{name: "evidence.get", call: func(_ context.Context, args json.RawMessage) (native.ToolResult, error) {
		seenArguments = args
		return native.ToolResult{JSON: json.RawMessage(`{"rows":[{"value":42}]}`)}, nil
	}}
	provider := providerRespondingWith(callingTools(toolCall("evidence.get", `{"entity_id":"motor-1"}`)))

	outcome := execute(t, newExecutor(t, provider, tool), map[string]any{"model_calls": 3})

	if outcome.Status != string(episodeledger.AttemptProduced) || len(outcome.DecisionJSON) == 0 {
		t.Fatalf("Execute() outcome = %+v, want a produced decision", outcome)
	}
	if string(seenArguments) != `{"entity_id":"motor-1"}` {
		t.Fatalf("tool arguments = %s", seenArguments)
	}
	observations := provider.request(2).Observations
	if len(observations) != 1 || observations[0].ToolName != "evidence.get" || observations[0].CallID != "call-evidence.get" ||
		string(observations[0].ResultJSON) != `{"rows":[{"value":42}]}` || observations[0].Bytes != uint64(len(`{"rows":[{"value":42}]}`)) {
		t.Fatalf("second model call observations = %+v", observations)
	}
	if len(provider.request(1).Observations) != 0 {
		t.Fatalf("the first model call already had observations: %+v", provider.request(1).Observations)
	}
}

func TestExecuteReportsAFailedToolAndAnEmptyResultAsObservations(t *testing.T) {
	t.Parallel()
	failing := toolFunc{name: "flaky", call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
		return native.ToolResult{}, errors.New("backend down")
	}}
	empty := toolFunc{name: "silent", call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
		return native.ToolResult{Bytes: 9}, nil
	}}
	provider := providerRespondingWith(callingTools(toolCall("flaky", `{}`), toolCall("silent", `{}`)))

	outcome := execute(t, newExecutor(t, provider, failing, empty), map[string]any{"model_calls": 3})

	if outcome.Status != string(episodeledger.AttemptProduced) {
		t.Fatalf("a failing tool must not fail the attempt: %+v", outcome)
	}
	observations := provider.request(2).Observations
	if len(observations) != 2 || observations[0].ErrorCode != "tool_failed" || len(observations[0].ResultJSON) != 0 {
		t.Fatalf("failed tool observation = %+v", observations)
	}
	if string(observations[1].ResultJSON) != "null" || observations[1].Bytes != 9 {
		t.Fatalf("empty result observation = %+v, want null counted at the tool's own byte report", observations[1])
	}
}

func TestExecuteAsksOnceForARepairOfAnInvalidDecision(t *testing.T) {
	t.Parallel()
	provider := providerRespondingWith(func(call int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		if call == 1 {
			return native.ModelResponse{DecisionJSON: []byte(`{"episode_id":"other"}`)}, nil
		}
		return validDecision(ctx, req)
	})

	outcome := execute(t, newExecutor(t, provider), map[string]any{"model_calls": 5})

	if outcome.Status != string(episodeledger.AttemptProduced) || provider.calls() != 2 {
		t.Fatalf("Execute() outcome = %+v after %d calls, want a repaired decision on the second call", outcome, provider.calls())
	}
	if first, second := provider.request(1), provider.request(2); first.Repair || !second.Repair || second.RepairReason != "decision_identity_mismatch" {
		t.Fatalf("repair flags: first %v, second %v with reason %q", first.Repair, second.Repair, second.RepairReason)
	}
}

func TestExecuteRetriesARetryableProviderErrorAfterABackoff(t *testing.T) {
	t.Parallel()
	var firstAt, secondAt time.Time
	provider := providerRespondingWith(func(call int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		if call == 1 {
			firstAt = time.Now()
			return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
		}
		secondAt = time.Now()
		return validDecision(ctx, req)
	})

	outcome := execute(t, newExecutor(t, provider), map[string]any{"model_calls": 3, "provider_retries": 1})

	if outcome.Status != string(episodeledger.AttemptProduced) || provider.calls() != 2 {
		t.Fatalf("Execute() outcome = %+v after %d calls, want one retry and a produced outcome", outcome, provider.calls())
	}
	if backoff := secondAt.Sub(firstAt); backoff < 10*time.Millisecond {
		t.Fatalf("retry came %s after the failure, want at least the 10ms backoff", backoff)
	}
}
