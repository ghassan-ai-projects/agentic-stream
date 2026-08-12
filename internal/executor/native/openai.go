package native

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// OpenAICompatibleProvider speaks the JSON/SSE subset shared by OpenAI-style
// chat-completions endpoints. It is deliberately a provider adapter only: it
// has no tool execution, credentials beyond its request header, or action
// access.
type OpenAICompatibleProvider struct {
	Endpoint string
	APIKey   string
	Model    string
	Client   *http.Client
}

// Name returns the provider adapter identity.
func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible" }

// Stream sends one bounded structured-output turn and normalizes either a
// regular JSON response or Server-Sent Events into ModelResponse.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if p == nil || strings.TrimSpace(p.Endpoint) == "" || strings.TrimSpace(p.Model) == "" {
		return ModelResponse{}, errors.New("openai-compatible endpoint and model are required")
	}
	body, err := json.Marshal(openAIRequest{Model: p.Model, Stream: true, Messages: []openAIMessage{
		{Role: "system", Content: req.Prompt},
		{Role: "user", Content: buildUserContent(req)},
	}, ResponseFormat: map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "decision", "strict": true, "schema": json.RawMessage(req.DecisionSchema)}}})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("marshal provider request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(body))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("create provider request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("call model provider: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		failure := fmt.Errorf("model provider returned %s: %s", response.Status, strings.TrimSpace(string(message)))
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return ModelResponse{}, &RetryableError{Err: failure}
		}
		return ModelResponse{}, failure
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return parseSSE(response.Body)
	}
	return parseJSONResponse(response.Body)
}

type openAIRequest struct {
	Model          string          `json:"model"`
	Stream         bool            `json:"stream"`
	Messages       []openAIMessage `json:"messages"`
	ResponseFormat map[string]any  `json:"response_format"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildUserContent(req ModelRequest) string {
	document := map[string]any{
		"objective":            req.Objective,
		"snapshot":             req.Snapshot,
		"allowed_intent_types": req.AllowedIntentTypes,
		"risk_ceiling":         req.RiskCeiling,
		"observations":         req.Observations,
	}
	if req.Repair {
		document["repair_reason"] = req.RepairReason
	}
	raw, _ := json.Marshal(document)
	return string(raw)
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
	Usage   struct {
		PromptTokens     uint64 `json:"prompt_tokens"`
		InputTokens      uint64 `json:"input_tokens"`
		CompletionTokens uint64 `json:"completion_tokens"`
		OutputTokens     uint64 `json:"output_tokens"`
		CostMicrounits   uint64 `json:"cost_microunits"`
	} `json:"usage"`
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
	result := ModelResponse{DecisionJSON: []byte(choice.Message.Content), Usage: responseUsage(response)}
	result.ToolCalls = normalizeToolCalls(choice.Message.ToolCalls)
	return result, nil
}

func parseSSE(reader io.Reader) (ModelResponse, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 16<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var content strings.Builder
	toolCalls := make(map[int]*ToolCall)
	var usage Usage
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
		usage = addUsage(usage, responseUsage(response))
		if len(response.Choices) == 0 {
			continue
		}
		choice := response.Choices[0]
		content.WriteString(choice.Delta.Content)
		for _, call := range choice.Delta.ToolCalls {
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
	if err := scanner.Err(); err != nil {
		return ModelResponse{}, fmt.Errorf("read model stream: %w", err)
	}
	result := ModelResponse{DecisionJSON: []byte(strings.TrimSpace(content.String())), Usage: usage}
	indices := make([]int, 0, len(toolCalls))
	for index := range toolCalls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		if call := toolCalls[index]; call != nil {
			result.ToolCalls = append(result.ToolCalls, *call)
		}
	}
	return result, nil
}

func normalizeToolCalls(calls []openAIToolCall) []ToolCall {
	result := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
	}
	return result
}

func responseUsage(response openAIResponse) Usage {
	input := response.Usage.InputTokens
	if input == 0 {
		input = response.Usage.PromptTokens
	}
	output := response.Usage.OutputTokens
	if output == 0 {
		output = response.Usage.CompletionTokens
	}
	return Usage{InputTokens: input, OutputTokens: output, CostMicrounits: response.Usage.CostMicrounits}
}
