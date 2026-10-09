package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestNewRejectsAnIncompleteConfiguration(t *testing.T) {
	t.Parallel()
	provider := providerRespondingWith(decidesImmediately)
	tests := []struct {
		name   string
		config native.Config
		want   string
	}{
		{"no provider", native.Config{}, "native provider is required"},
		{"nil tool", native.Config{Provider: provider, Tools: []native.Tool{nil}}, "native tool name is required"},
		{"unnamed tool", native.Config{Provider: provider, Tools: []native.Tool{toolReturning(" ", `{}`)}}, "native tool name is required"},
		{"duplicate tool", native.Config{Provider: provider, Tools: []native.Tool{toolReturning("evidence.get", `{}`), toolReturning("evidence.get", `{}`)}}, `duplicate native tool "evidence.get"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if executor, err := native.New(tt.config); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("native.New() = %v, %v; want error containing %q", executor, err, tt.want)
			}
		})
	}
}

func TestExecuteRefusesWhatItCannotRun(t *testing.T) {
	t.Parallel()
	var unconfigured *native.Executor
	tests := []struct {
		name     string
		executor *native.Executor
		req      *episodes.Request
		want     string
	}{
		{"nil executor", unconfigured, requestWithBudget(nil), "native executor is not configured"},
		{"nil request", newExecutor(t, providerRespondingWith(decidesImmediately)), nil, "episode request is required"},
		{"request that is not json", newExecutor(t, providerRespondingWith(decidesImmediately)), &episodes.Request{RequestJSON: []byte("{")}, "decode native episode request"},
		{"request without a snapshot", newExecutor(t, providerRespondingWith(decidesImmediately)), &episodes.Request{RequestJSON: []byte(`{"budget":{"model_calls":1}}`)}, "requires snapshot and decision schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if outcome, err := tt.executor.Execute(t.Context(), tt.req); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() = %+v, %v; want error containing %q", outcome, err, tt.want)
			}
		})
	}
}

func TestToolFactoryToolsAreBuiltFromTheTrustedRequest(t *testing.T) {
	t.Parallel()
	var factoryRequests []string
	var called bool
	scoped := toolFunc{name: "evidence.get", call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
		called = true
		return native.ToolResult{JSON: json.RawMessage(`{"rows":[]}`)}, nil
	}}
	executor, err := native.New(native.Config{
		Provider: providerRespondingWith(callingTools(toolCall("evidence.get", `{}`))),
		ToolFactory: func(req *episodes.Request) []native.Tool {
			factoryRequests = append(factoryRequests, req.AttemptID)
			return []native.Tool{scoped, nil, toolReturning(" ", `{}`)}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := requestWithBudget(map[string]any{"model_calls": 3})

	outcome, err := executor.Execute(t.Context(), req)

	if err != nil || outcome.Status != "produced" || !called {
		t.Fatalf("Execute() = %+v, %v (tool called: %v); want a produced outcome through the factory's tool", outcome, err, called)
	}
	if len(factoryRequests) != 1 || factoryRequests[0] != req.AttemptID {
		t.Fatalf("factory requests = %v, want one call for the attempt %q", factoryRequests, req.AttemptID)
	}
}
