package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
)

type providerStream struct {
	content       strings.Builder
	toolCalls     map[int]*domain.ToolCall
	usage         domain.Usage
	usageReported bool
}

func parseSSE(reader io.Reader) (domain.ModelResponse, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 16<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	stream := providerStream{toolCalls: make(map[int]*domain.ToolCall)}
	for scanner.Scan() {
		done, err := stream.acceptLine(scanner.Text())
		if err != nil {
			return domain.ModelResponse{}, err
		}
		if done {
			break
		}
	}
	return stream.finish(scanner.Err())
}

func (s *providerStream) acceptLine(raw string) (bool, error) {
	line := strings.TrimSpace(raw)
	if !strings.HasPrefix(line, "data:") {
		return false, nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "[DONE]" {
		return true, nil
	}
	var response openAIResponse
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		return false, fmt.Errorf("decode model stream event: %w", err)
	}
	s.acceptChunk(response)
	return false, nil
}

func (s *providerStream) acceptChunk(response openAIResponse) {
	chunkUsage, reported := responseUsage(response)
	s.usage = domain.AddUsage(s.usage, chunkUsage)
	s.usageReported = s.usageReported || reported
	if len(response.Choices) == 0 {
		return
	}
	s.content.WriteString(response.Choices[0].Delta.Content)
	appendToolCallDeltas(s.toolCalls, response.Choices[0].Delta.ToolCalls)
}

func (stream *providerStream) finish(err error) (domain.ModelResponse, error) {
	if err != nil {
		return domain.ModelResponse{}, fmt.Errorf("read model stream: %w", err)
	}
	return stream.response(), nil
}

func (s *providerStream) response() domain.ModelResponse {
	return domain.ModelResponse{DecisionJSON: []byte(strings.TrimSpace(s.content.String())), Usage: s.usage, UsageReported: s.usageReported, ToolCalls: orderedToolCalls(s.toolCalls)}
}

func appendToolCallDeltas(toolCalls map[int]*domain.ToolCall, deltas []openAIToolCall) {
	for _, call := range deltas {
		index := call.Index
		if toolCalls[index] == nil {
			toolCalls[index] = &domain.ToolCall{ID: call.ID, Name: call.Function.Name}
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

func orderedToolCalls(toolCalls map[int]*domain.ToolCall) []domain.ToolCall {
	var result []domain.ToolCall
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
