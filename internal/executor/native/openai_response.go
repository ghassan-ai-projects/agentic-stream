package native

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func checkProviderStatus(response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		failure := fmt.Errorf("model provider returned %s: %s", response.Status, strings.TrimSpace(string(message)))
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return &RetryableError{Err: failure}
		}
		return failure
	}
	return nil
}

func decodeProviderResponse(response *http.Response) (ModelResponse, error) {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		parsed, err := parseSSE(response.Body)
		if err != nil {
			return ModelResponse{}, err
		}
		if !parsed.UsageReported {
			return ModelResponse{}, errors.New("model provider stream omitted usage")
		}
		return parsed, nil
	}
	parsed, err := parseJSONResponse(response.Body)
	if err != nil {
		return ModelResponse{}, err
	}
	if !parsed.UsageReported {
		return ModelResponse{}, errors.New("model provider response omitted usage")
	}
	return parsed, nil
}

type openAIChoice struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []openAIToolCall `json:"tool_calls"`
	} `json:"message"`
	Delta struct {
		Content   string           `json:"content"`
		ToolCalls []openAIToolCall `json:"tool_calls"`
	} `json:"delta"`
}

type openAIToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   *struct {
		PromptTokens     uint64 `json:"prompt_tokens"`
		InputTokens      uint64 `json:"input_tokens"`
		CompletionTokens uint64 `json:"completion_tokens"`
		OutputTokens     uint64 `json:"output_tokens"`
		CostMicrounits   uint64 `json:"cost_microunits"`
	} `json:"usage,omitempty"`
}

func parseJSONResponse(reader io.Reader) (ModelResponse, error) {
	var response openAIResponse
	if err := json.NewDecoder(io.LimitReader(reader, 16<<20)).Decode(&response); err != nil {
		return ModelResponse{}, fmt.Errorf("decode model response: %w", err)
	}
	if len(response.Choices) == 0 {
		return ModelResponse{}, errors.New("model response contains no choices")
	}
	choice := response.Choices[0]
	usage, reported := responseUsage(response)
	result := ModelResponse{DecisionJSON: []byte(choice.Message.Content), Usage: usage, UsageReported: reported}
	result.ToolCalls = normalizeToolCalls(choice.Message.ToolCalls)
	return result, nil
}

func normalizeToolCalls(calls []openAIToolCall) []ToolCall {
	result := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
	}
	return result
}

func responseUsage(response openAIResponse) (Usage, bool) {
	if response.Usage == nil {
		return Usage{}, false
	}
	input := response.Usage.InputTokens
	if input == 0 {
		input = response.Usage.PromptTokens
	}
	output := response.Usage.OutputTokens
	if output == 0 {
		output = response.Usage.CompletionTokens
	}
	return Usage{InputTokens: input, OutputTokens: output, CostMicrounits: response.Usage.CostMicrounits}, true
}
