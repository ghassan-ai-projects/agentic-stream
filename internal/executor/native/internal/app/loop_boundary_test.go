package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
)

func TestFailedToolCallCannotBeRepeated(t *testing.T) {
	t.Parallel()
	calls := 0
	loop := episodeLoop{req: &episodes.Request{AttemptID: "attempt", Fence: 2}, tools: map[string]domain.Tool{"read": failingReadTool{calls: &calls}}, seenCalls: make(map[string]struct{})}
	call := domain.ToolCall{ID: "first", Name: "read", Arguments: json.RawMessage(`{}`)}
	if outcome := loop.runTool(t.Context(), call); outcome != nil {
		t.Fatalf("first failure should become observation: %#v", outcome)
	}
	if len(loop.observations) != 1 || loop.observations[0].ErrorCode != "tool_failed" {
		t.Fatalf("failure observation: %#v", loop.observations)
	}
	call.ID = "second"
	outcome := loop.runTool(t.Context(), call)
	if outcome == nil || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "repeated_tool_call:read" {
		t.Fatalf("repeat outcome: %#v", outcome)
	}
	if calls != 1 {
		t.Fatalf("tool executed %d times", calls)
	}
}

type failingReadTool struct{ calls *int }

func (f failingReadTool) Name() string { return "read" }
func (f failingReadTool) Call(context.Context, json.RawMessage) (domain.ToolResult, error) {
	*f.calls++
	return domain.ToolResult{}, errors.New("read domain.Failed")
}

func TestToolResultBudgetAccountsRejectedBytes(t *testing.T) {
	t.Parallel()
	loop := episodeLoop{executor: &Executor{}, req: &episodes.Request{AttemptID: "attempt"}, budget: domain.Budget{TotalToolResultBytes: 4}, toolResultBytes: 3}
	outcome := loop.recordToolResult(t.Context(), domain.ToolCall{ID: "call", Name: "read"}, domain.ToolResult{JSON: []byte(`{}`)})
	if outcome == nil || outcome.Reasons[0] != "budget_exhausted:total_tool_result_bytes" {
		t.Fatalf("result: %#v", outcome)
	}
	if loop.toolResultBytes != 5 || len(loop.observations) != 0 {
		t.Fatalf("bytes=%d observations=%v", loop.toolResultBytes, loop.observations)
	}
}
