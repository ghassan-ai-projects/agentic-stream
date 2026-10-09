package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

func TestExecuteNeverCallsTheProviderOnACanceledContext(t *testing.T) {
	t.Parallel()
	provider := providerRespondingWith(decidesImmediately)
	if err := executorconformance.RunCanceled(t.Context(), newExecutor(t, provider)); err != nil {
		t.Fatal(err)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider calls = %d, want 0 once the context is canceled", provider.calls())
	}
}

func TestExecuteSettlesALateProviderResponseAfterTheWallTime(t *testing.T) {
	t.Parallel()
	provider := providerRespondingWith(func(_ int, ctx context.Context, req native.ModelRequest) (native.ModelResponse, error) {
		<-ctx.Done()
		response, err := validDecision(ctx, req)
		response.Usage = native.Usage{CostMicrounits: 42}
		return response, err
	})
	req := requestWithBudget(map[string]any{"wall_time": "1ms"})

	outcome, err := newExecutor(t, provider).Execute(t.Context(), req)

	if err != nil || outcome.CostMicrounits != 42 {
		t.Fatalf("Execute() = %+v, %v; want the late response's cost settled", outcome, err)
	}
	if err := executorconformance.CheckContextEnding(req, outcome, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
}

func TestExecutePreservesUsageWhenCanceledDuringALaterTurn(t *testing.T) {
	t.Parallel()
	secondTurn := make(chan struct{})
	provider := providerRespondingWith(func(call int, ctx context.Context, _ native.ModelRequest) (native.ModelResponse, error) {
		if call == 1 {
			return native.ModelResponse{Usage: native.Usage{CostMicrounits: 42}, ToolCalls: []native.ToolCall{toolCall("evidence.get", `{}`)}}, nil
		}
		close(secondTurn)
		<-ctx.Done()
		return native.ModelResponse{}, fmt.Errorf("provider interrupted: %w", ctx.Err())
	})
	executor := newExecutor(t, provider, toolReturning("evidence.get", `{"rows":[]}`))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := executeInBackground(ctx, executor, requestWithBudget(map[string]any{"model_calls": 3}))

	awaitSignal(t, secondTurn, "the provider reached its second turn")
	cancel()
	completed := <-result

	if completed.err != nil || completed.outcome.Status != string(episodeledger.AttemptCancelled) || completed.outcome.CostMicrounits != 42 {
		t.Fatalf("Execute() = %+v, %v; want a canceled attempt that kept the 42 microunits already consumed", completed.outcome, completed.err)
	}
}

func TestExecuteStopsRetryingWhenCanceledDuringTheBackoff(t *testing.T) {
	t.Parallel()
	firstCall := make(chan struct{})
	provider := providerRespondingWith(func(call int, _ context.Context, _ native.ModelRequest) (native.ModelResponse, error) {
		if call == 1 {
			close(firstCall)
		}
		return native.ModelResponse{}, &native.RetryableError{Err: errors.New("temporary")}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := executeInBackground(ctx, newExecutor(t, provider), requestWithBudget(map[string]any{"model_calls": 5, "provider_retries": 3}))

	awaitSignal(t, firstCall, "the provider received its first call")
	cancel()
	completed := <-result

	if completed.err != nil || completed.outcome.Status != string(episodeledger.AttemptCancelled) {
		t.Fatalf("Execute() = %+v, %v; want a canceled attempt", completed.outcome, completed.err)
	}
	if provider.calls() != 1 {
		t.Fatalf("provider calls = %d, want no retry after cancellation", provider.calls())
	}
}
