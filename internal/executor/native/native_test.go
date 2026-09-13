package native_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"tool_result_bytes":4,"model_calls":3}`)
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
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"provider_retries":1,"model_calls":3}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodes.AttemptProduced) || provider.calls != 2 {
		t.Fatalf("expected one retry and a produced outcome, calls=%d outcome=%#v", provider.calls, outcome)
	}
}

func TestNativeExecutorEnforcesReportedUsageBudgets(t *testing.T) {
	for _, test := range []struct {
		name  string
		usage native.Usage
		limit string
		want  string
	}{
		{name: "input tokens", usage: native.Usage{InputTokens: 11}, limit: `{"input_tokens":10,"model_calls":3}`, want: "budget_exhausted:input_tokens"},
		{name: "output tokens", usage: native.Usage{OutputTokens: 11}, limit: `{"output_tokens":10,"model_calls":3}`, want: "budget_exhausted:output_tokens"},
		{name: "cost", usage: native.Usage{CostMicrounits: 11}, limit: `{"cost_microunits":10,"model_calls":3}`, want: "budget_exhausted:cost_microunits"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor, err := native.New(native.Config{Provider: usageProvider{usage: test.usage}})
			if err != nil {
				t.Fatal(err)
			}
			request := conformance.FixtureRequest()
			request.RequestJSON = replaceBudget(request.RequestJSON, test.limit)
			outcome, err := executor.Execute(context.Background(), request)
			if err != nil || outcome.Status != string(episodes.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != test.want {
				t.Fatalf("outcome=%+v err=%v", outcome, err)
			}
		})
	}
}

func TestNativeExecutorEnforcesWallTimeAfterLateProviderResponse(t *testing.T) {
	executor, err := native.New(native.Config{Provider: usageProvider{usage: native.Usage{CostMicrounits: 42}, delay: 10 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"wall_time":"1ms"}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil || outcome.Status != string(episodes.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "timed_out" || outcome.CostMicrounits != 42 {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestNativeExecutorRejectsUnboundedRequest(t *testing.T) {
	provider := &countingProvider{}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{}`)
	if _, err := executor.Execute(context.Background(), request); err == nil {
		t.Fatal("unbounded request succeeded")
	} else if !strings.Contains(err.Error(), "finite episode budget requires wall_time or model_calls") {
		t.Fatalf("unbounded request error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 for unbounded request", provider.calls)
	}
}

func TestNativeExecutorRejectsMissingUsageForUsageBound(t *testing.T) {
	executor, err := native.New(native.Config{Provider: missingUsageProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"input_tokens":10,"model_calls":1}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outcome.Status != string(episodes.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "budget_telemetry_missing" {
		t.Fatalf("outcome=%+v, want missing telemetry failure", outcome)
	}
}

func TestNativeExecutorPreservesUsageOnCancellation(t *testing.T) {
	secondStarted := make(chan struct{})
	provider := &usageThenCancelProvider{secondStarted: secondStarted}
	executor, err := native.New(native.Config{
		Provider: provider,
		Tools: []native.Tool{toolFunc{name: "evidence.get", call: func(context.Context, json.RawMessage) (native.ToolResult, error) {
			return native.ToolResult{JSON: json.RawMessage(`{"rows":[]}`)}, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"model_calls":3}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan struct {
		outcome *episodes.Outcome
		err     error
	}, 1)
	go func() {
		outcome, executeErr := executor.Execute(ctx, request)
		result <- struct {
			outcome *episodes.Outcome
			err     error
		}{outcome: outcome, err: executeErr}
	}()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("provider did not reach the cancellable second turn")
	}
	cancel()
	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatalf("canceled execution error = %v", completed.err)
		}
		if completed.outcome.Status != string(episodes.AttemptCancelled) || completed.outcome.CostMicrounits != 42 {
			t.Fatalf("canceled outcome = %+v, want canceled with cost 42", completed.outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled execution did not finish")
	}
}

func TestNativeExecutorBacksOffProviderRetries(t *testing.T) {
	provider := &timedRetryProvider{}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"model_calls":3,"provider_retries":1}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil || outcome.Status != string(episodes.AttemptProduced) {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if provider.calls != 2 || provider.secondAt.Sub(provider.firstAt) < 8*time.Millisecond {
		t.Fatalf("retry calls=%d delay=%s, want two calls with bounded backoff", provider.calls, provider.secondAt.Sub(provider.firstAt))
	}
}

func TestNativeExecutorCapsProviderRetries(t *testing.T) {
	provider := &alwaysRetryProvider{}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"model_calls":5,"provider_retries":1}`)
	outcome, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("capped retries returned error: %v", err)
	}
	if outcome.Status != string(episodes.AttemptFailed) || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "budget_exhausted:provider_retries" {
		t.Fatalf("capped retry outcome=%+v, want retry budget failure", outcome)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls=%d, want initial call plus one retry", provider.calls)
	}
}

func TestNativeExecutorCancellationStopsRetryBackoff(t *testing.T) {
	provider := &cancelDuringRetryProvider{firstCall: make(chan struct{})}
	executor, err := native.New(native.Config{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	request.RequestJSON = replaceBudget(request.RequestJSON, `{"model_calls":5,"provider_retries":3}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan struct {
		outcome *episodes.Outcome
		err     error
	}, 1)
	go func() {
		outcome, executeErr := executor.Execute(ctx, request)
		result <- struct {
			outcome *episodes.Outcome
			err     error
		}{outcome: outcome, err: executeErr}
	}()
	select {
	case <-provider.firstCall:
	case <-time.After(time.Second):
		t.Fatal("retry provider did not receive its first call")
	}
	cancel()
	select {
	case completed := <-result:
		if completed.err != nil || completed.outcome.Status != string(episodes.AttemptCancelled) {
			t.Fatalf("canceled retry outcome=%+v err=%v, want canceled outcome", completed.outcome, completed.err)
		}
		if provider.calls != 1 {
			t.Fatalf("provider calls=%d, want no retry after cancellation", provider.calls)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled retry did not finish")
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

type usageProvider struct {
	usage native.Usage
	delay time.Duration
}

func (p usageProvider) Name() string { return "usage" }
func (p usageProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	response, err := (&native.DeterministicProvider{}).Stream(ctx, req)
	response.Usage = p.usage
	if err != nil {
		return response, fmt.Errorf("deterministic provider: %w", err)
	}
	return response, nil
}

type countingProvider struct{ calls int }

func (p *countingProvider) Name() string { return "counting" }
func (p *countingProvider) Stream(context.Context, native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	return native.ModelResponse{}, nil
}

type missingUsageProvider struct{}

func (missingUsageProvider) Name() string { return "missing-usage" }
func (missingUsageProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	response, err := (&native.DeterministicProvider{}).Stream(ctx, req)
	if err != nil {
		return native.ModelResponse{}, fmt.Errorf("deterministic stream: %w", err)
	}
	response.Usage = native.Usage{}
	response.UsageReported = false
	return response, nil
}

type usageThenCancelProvider struct {
	secondStarted chan<- struct{}
	calls         int
}

func (p *usageThenCancelProvider) Name() string { return "usage-then-cancel" }
func (p *usageThenCancelProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	if p.calls == 1 {
		return native.ModelResponse{
			Usage:     native.Usage{CostMicrounits: 42},
			ToolCalls: []native.ToolCall{{ID: "call-1", Name: "evidence.get", Arguments: json.RawMessage(`{}`)}},
		}, nil
	}
	close(p.secondStarted)
	<-ctx.Done()
	return native.ModelResponse{}, fmt.Errorf("usage provider canceled: %w", ctx.Err())
}

type timedRetryProvider struct {
	calls    int
	firstAt  time.Time
	secondAt time.Time
}

type alwaysRetryProvider struct{ calls int }

func (p *alwaysRetryProvider) Name() string { return "always-retry" }
func (p *alwaysRetryProvider) Stream(context.Context, native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
}

type cancelDuringRetryProvider struct {
	firstCall chan struct{}
	calls     int
}

func (p *cancelDuringRetryProvider) Name() string { return "cancel-during-retry" }
func (p *cancelDuringRetryProvider) Stream(context.Context, native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	if p.calls == 1 {
		close(p.firstCall)
	}
	return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
}

func (p *timedRetryProvider) Name() string { return "timed-retry" }
func (p *timedRetryProvider) Stream(ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
	p.calls++
	if p.calls == 1 {
		p.firstAt = time.Now()
		return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
	}
	p.secondAt = time.Now()
	response, err := (&native.DeterministicProvider{}).Stream(ctx, req)
	if err != nil {
		return native.ModelResponse{}, fmt.Errorf("deterministic retry response: %w", err)
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
