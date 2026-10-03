package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

// runTools executes the requested tool calls as observations for the next
// model call. Only allow-listed tools with valid, never-repeated arguments
// run, within the tool-call and result-byte budgets.
func (l *episodeLoop) runTools(ctx context.Context, calls []ToolCall) *episodes.Outcome {
	if l.budget.ToolCalls > 0 && uint32(len(calls)) > l.budget.ToolCalls-l.toolCalls { //nolint:gosec // bounded by provider response and checked below.
		return failed(l.req, "budget_exhausted:tool_calls", l.usage)
	}
	for _, call := range calls {
		l.toolCalls++
		if err := ctx.Err(); err != nil {
			return terminalForContext(l.req, err, l.usage)
		}
		if outcome := l.runTool(ctx, call); outcome != nil {
			return outcome
		}
	}
	return nil
}

func (l *episodeLoop) runTool(ctx context.Context, call ToolCall) *episodes.Outcome {
	tool, ok := l.tools[call.Name]
	if !ok {
		return failed(l.req, "tool_not_allowed:"+call.Name, l.usage)
	}
	if outcome := l.admitToolCall(call); outcome != nil {
		return outcome
	}
	result, err := tool.Call(ctx, call.Arguments)
	if err != nil {
		return l.recordToolFailure(call, err)
	}
	return l.recordToolResult(ctx, call, result)
}

func (l *episodeLoop) admitToolCall(call ToolCall) *episodes.Outcome {
	if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
		return failed(l.req, "tool_arguments_invalid:"+call.Name, l.usage)
	}
	digest := sha256.Sum256(append([]byte(call.Name+"|"), call.Arguments...))
	key := hex.EncodeToString(digest[:])
	if _, exists := l.seenCalls[key]; exists {
		return failed(l.req, "repeated_tool_call:"+call.Name, l.usage)
	}
	l.seenCalls[key] = struct{}{}
	return nil
}

func (l *episodeLoop) recordToolFailure(call ToolCall, err error) *episodes.Outcome {
	if errors.Is(err, ErrInterrupt) {
		return failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
	}
	l.observations = append(l.observations, Observation{CallID: call.ID, ToolName: call.Name, ErrorCode: "tool_failed"})
	return nil
}

func (l *episodeLoop) recordToolResult(ctx context.Context, call ToolCall, result ToolResult) *episodes.Outcome {
	observation, err := l.executor.observe(ctx, call, result, l.budget)
	if err != nil {
		return failed(l.req, err.Error(), l.usage)
	}
	l.toolResultBytes += observation.Bytes
	if l.budget.TotalToolResultBytes > 0 && l.toolResultBytes > l.budget.TotalToolResultBytes {
		return failed(l.req, "budget_exhausted:total_tool_result_bytes", l.usage)
	}
	l.observations = append(l.observations, observation)
	return nil
}
