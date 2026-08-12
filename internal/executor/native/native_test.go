package native_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/conformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestDeterministicNativeExecutorConforms(t *testing.T) {
	executor, err := native.New(native.Config{Provider: &native.DeterministicProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.Run(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
}

func TestNativeExecutorRunsReadToolThenDecision(t *testing.T) {
	provider := &native.DeterministicProvider{Responses: []native.ModelResponse{{ToolCalls: []native.ToolCall{{ID: "call-1", Name: "evidence.get", Arguments: json.RawMessage(`{"entity_id":"motor-1"}`)}}}}}
	store := native.NewMemoryArtifactStore()
	executor, err := native.New(native.Config{Provider: provider, ArtifactStore: store, Tools: []native.Tool{toolFunc{name: "evidence.get", call: func(_ context.Context, _ json.RawMessage) (native.ToolResult, error) {
		return native.ToolResult{JSON: json.RawMessage(`{"rows":[{"value":42}]}`)}, nil
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executor.Execute(context.Background(), conformance.FixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodes.AttemptProduced) {
		t.Fatalf("expected produced outcome, got %#v", outcome)
	}
	if len(outcome.DecisionJSON) == 0 {
		t.Fatal("expected decision")
	}
}

func TestNativeExecutorSpillsOversizedResult(t *testing.T) {
	provider := &native.DeterministicProvider{Responses: []native.ModelResponse{{ToolCalls: []native.ToolCall{{ID: "call-1", Name: "evidence.get", Arguments: json.RawMessage(`{}`)}}}}}
	store := native.NewMemoryArtifactStore()
	executor, err := native.New(native.Config{Provider: provider, ArtifactStore: store, Tools: []native.Tool{toolFunc{name: "evidence.get", call: func(_ context.Context, _ json.RawMessage) (native.ToolResult, error) {
		return native.ToolResult{JSON: json.RawMessage(`{"large":"payload"}`)}, nil
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"tool_result_bytes":4}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodes.AttemptProduced) {
		t.Fatalf("expected artifact-backed continuation, got %#v", outcome)
	}
}

func TestNativeExecutorFailsInterruptImmediately(t *testing.T) {
	provider := interruptProvider{}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executor.Execute(context.Background(), conformance.FixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodes.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "interrupt_in_non_interactive_episode" {
		t.Fatalf("unexpected interrupt outcome: %#v", outcome)
	}
}

func TestNativeExecutorUsesBoundedProviderRetry(t *testing.T) {
	provider := &retryProvider{}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"provider_retries":1}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodes.AttemptProduced) || provider.calls != 2 {
		t.Fatalf("expected one retry and a produced outcome, calls=%d outcome=%#v", provider.calls, outcome)
	}
}

type toolFunc struct {
	name string
	call func(context.Context, json.RawMessage) (native.ToolResult, error)
}

func (t toolFunc) Name() string { return t.name }
func (t toolFunc) Call(ctx context.Context, args json.RawMessage) (native.ToolResult, error) {
	return t.call(ctx, args)
}

type interruptProvider struct{}

func (interruptProvider) Name() string { return "interrupt" }
func (interruptProvider) Stream(context.Context, native.ModelRequest) (native.ModelResponse, error) {
	return native.ModelResponse{}, native.ErrInterrupt
}

type retryProvider struct{ calls int }

func (p *retryProvider) Name() string { return "retry" }
func (p *retryProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	if p.calls == 1 {
		return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
	}
	response, err := (&native.DeterministicProvider{}).Stream(ctx, req)
	if err != nil {
		return native.ModelResponse{}, fmt.Errorf("deterministic provider failed: %w", err)
	}
	return response, nil
}

func replaceBudget(raw []byte, budget string) []byte {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		panic(err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(budget), &value); err != nil {
		panic(err)
	}
	document["budget"] = value
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return encoded
}
