package app_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

type respondFunc func(call int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error)

type scriptedProvider struct {
	respond  respondFunc
	mu       sync.Mutex
	requests []native.ModelRequest
}

func providerRespondingWith(respond respondFunc) *scriptedProvider {
	return &scriptedProvider{respond: respond}
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	call := len(p.requests)
	p.mu.Unlock()
	return p.respond(call, ctx, req)
}

func (p *scriptedProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func (p *scriptedProvider) request(call int) native.ModelRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[call-1]
}

func validDecision(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	response, err := (&native.DeterministicProvider{}).Stream(ctx, req)
	if err != nil {
		return native.ModelResponse{}, err //nolint:wrapcheck // The deterministic provider is the reference answer.
	}
	return response, nil
}

func decidesImmediately(_ int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	return validDecision(ctx, req)
}

func withUsage(usage native.Usage) respondFunc {
	return func(_ int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		response, err := validDecision(ctx, req)
		response.Usage = usage
		return response, err
	}
}

func failingWith(err error) respondFunc {
	return func(int, context.Context, native.ModelRequest) (native.ModelResponse, error) {
		return native.ModelResponse{}, err
	}
}

func callingTools(calls ...native.ToolCall) respondFunc {
	return func(call int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		if call == 1 {
			return native.ModelResponse{ToolCalls: calls}, nil
		}
		return validDecision(ctx, req)
	}
}

func toolCall(name, arguments string) native.ToolCall {
	return native.ToolCall{ID: "call-" + name, Name: name, Arguments: json.RawMessage(arguments)}
}

type toolFunc struct {
	name string
	call func(context.Context, json.RawMessage) (native.ToolResult, error)
}

func (t toolFunc) Name() string { return t.name }

func (t toolFunc) Call(ctx context.Context, args json.RawMessage) (native.ToolResult, error) {
	return t.call(ctx, args)
}

func toolReturning(name, result string) toolFunc {
	return toolFunc{name: name, call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
		return native.ToolResult{JSON: json.RawMessage(result)}, nil
	}}
}

func newExecutor(t *testing.T, provider native.ModelProvider, tools ...native.Tool) *native.Executor {
	t.Helper()
	executor, err := native.New(native.Config{Provider: provider, Tools: tools})
	if err != nil {
		t.Fatalf("native.New() = %v", err)
	}
	return executor
}

func requestWithBudget(budget map[string]any) *episodes.Request {
	req := executorconformance.FixtureRequest()
	if budget != nil {
		executorconformance.SetBudget(req, budget)
	}
	return req
}

func execute(t *testing.T, executor *native.Executor, budget map[string]any) *episodes.Outcome {
	t.Helper()
	outcome, err := executor.Execute(t.Context(), requestWithBudget(budget))
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	return outcome
}

func awaitSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-t.Context().Done():
		t.Fatalf("test ended before %s", what)
	}
}

type executed struct {
	outcome *episodes.Outcome
	err     error
}

func executeInBackground(ctx context.Context, executor *native.Executor, req *episodes.Request) <-chan executed {
	result := make(chan executed, 1)
	go func() {
		outcome, err := executor.Execute(ctx, req)
		result <- executed{outcome: outcome, err: err}
	}()
	return result
}
