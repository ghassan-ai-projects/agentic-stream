package native

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

func parseSSE(reader io.Reader) (ModelResponse, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 16<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var content strings.Builder
	toolCalls := make(map[int]*ToolCall)
	var usage Usage
	var usageReported bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var response openAIResponse
		if err := json.Unmarshal([]byte(data), &response); err != nil {
			return ModelResponse{}, fmt.Errorf("decode model stream event: %w", err)
		}
		chunkUsage, reported := responseUsage(response)
		usage = addUsage(usage, chunkUsage)
		usageReported = usageReported || reported
		if len(response.Choices) == 0 {
			continue
		}
		content.WriteString(response.Choices[0].Delta.Content)
		appendToolCallDeltas(toolCalls, response.Choices[0].Delta.ToolCalls)
	}
	if err := scanner.Err(); err != nil {
		return ModelResponse{}, fmt.Errorf("read model stream: %w", err)
	}
	result := ModelResponse{DecisionJSON: []byte(strings.TrimSpace(content.String())), Usage: usage, UsageReported: usageReported}
	result.ToolCalls = orderedToolCalls(toolCalls)
	return result, nil
}

func appendToolCallDeltas(toolCalls map[int]*ToolCall, deltas []openAIToolCall) {
	for _, call := range deltas {
		index := call.Index
		if toolCalls[index] == nil {
			toolCalls[index] = &ToolCall{ID: call.ID, Name: call.Function.Name}
		}
		if call.ID != "" {
			toolCalls[index].ID = call.ID
		}
		if call.Function.Name != "" {
			toolCalls[index].Name = call.Function.Name
		}
		toolCalls[index].Arguments = append(toolCalls[index].Arguments, []byte(call.Function.Arguments)...)
	}
}

func orderedToolCalls(toolCalls map[int]*ToolCall) []ToolCall {
	var result []ToolCall
	indices := make([]int, 0, len(toolCalls))
	for index := range toolCalls {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	for _, index := range indices {
		if call := toolCalls[index]; call != nil {
			result = append(result, *call)
		}
	}
	return result
}
