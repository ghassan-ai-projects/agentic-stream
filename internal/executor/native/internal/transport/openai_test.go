package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

const eventStream = "text/event-stream"

func reply(status int, contentType, body string) roundTripFunc {
	return func(*http.Request) (*http.Response, error) {
		header := make(http.Header)
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
		return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
}

func providerOver(transport http.RoundTripper) *native.OpenAICompatibleProvider {
	return &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: transport},
	}
}

func modelRequest() native.ModelRequest {
	return native.ModelRequest{Prompt: "system prompt", Objective: "diagnose", DecisionSchema: json.RawMessage(`{"type":"object"}`)}
}

func TestStreamSendsABoundedStructuredRequest(t *testing.T) {
	t.Parallel()
	var captured *http.Request
	var body map[string]any
	provider := providerOver(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
		return reply(http.StatusOK, "", `{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":1}}`)(req)
	}))
	provider.APIKey = "secret-key"
	request := modelRequest()
	request.Repair, request.RepairReason = true, "intent_not_allowed:x"
	request.AllowedIntentTypes, request.RiskCeiling = []string{"ticket"}, "R1"
	request.Tools = []native.ToolDefinition{{Name: "evidence_get", Description: "read evidence", Parameters: json.RawMessage(`{"type":"object"}`)}}
	if _, err := provider.Stream(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	if captured.Method != http.MethodPost || captured.Header.Get("Content-Type") != "application/json" || captured.Header.Get("Authorization") != "Bearer secret-key" {
		t.Fatalf("request = %s %v", captured.Method, captured.Header)
	}
	messages := body["messages"].([]any)
	system, user := messages[0].(map[string]any), messages[1].(map[string]any)
	if body["model"] != "test-model" || body["stream"] != true || body["stream_options"].(map[string]any)["include_usage"] != true || system["role"] != "system" || system["content"] != "system prompt" {
		t.Fatalf("body = %v", body)
	}
	for _, want := range []string{`"objective":"diagnose"`, `"allowed_intent_types":["ticket"]`, `"risk_ceiling":"R1"`, `"repair_reason":"intent_not_allowed:x"`} {
		if !strings.Contains(user["content"].(string), want) {
			t.Errorf("user content %s lacks %s", user["content"], want)
		}
	}
	tools := body["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" || tools[0].(map[string]any)["function"].(map[string]any)["name"] != "evidence_get" {
		t.Fatalf("tools = %v", body["tools"])
	}
	format := body["response_format"].(map[string]any)["json_schema"].(map[string]any)
	if format["strict"] != true || format["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("response_format = %v", body["response_format"])
	}
}

func TestStreamSendsNoCredentialWithoutAnAPIKeyAndNoRepairReasonUnlessRepairing(t *testing.T) {
	t.Parallel()
	var captured *http.Request
	var content string
	provider := providerOver(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
		content = body.Messages[1].Content
		return reply(http.StatusOK, "", `{"choices":[{"message":{"content":"{}"}}],"usage":{}}`)(req)
	}))
	if _, err := provider.Stream(t.Context(), modelRequest()); err != nil {
		t.Fatal(err)
	}
	if captured.Header.Get("Authorization") != "" || strings.Contains(content, "repair_reason") {
		t.Fatalf("authorization=%q content=%s", captured.Header.Get("Authorization"), content)
	}
}

func TestStreamDecodesAJSONResponse(t *testing.T) {
	t.Parallel()
	body := `{"choices":[{"message":{"content":"{\"ok\":true}","tool_calls":[{"id":"call-1","function":{"name":"evidence_get","arguments":"{\"limit\":1}"}}]}}],
		"usage":{"prompt_tokens":2,"completion_tokens":3,"cost_microunits":4}}`
	response, err := providerOver(reply(http.StatusOK, "application/json", body)).Stream(t.Context(), modelRequest())
	if err != nil {
		t.Fatal(err)
	}
	if string(response.DecisionJSON) != `{"ok":true}` || len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "call-1" ||
		response.ToolCalls[0].Name != "evidence_get" || string(response.ToolCalls[0].Arguments) != `{"limit":1}` {
		t.Fatalf("response = %+v", response)
	}
	if !response.UsageReported || response.Usage.InputTokens != 2 || response.Usage.OutputTokens != 3 || response.Usage.CostMicrounits != 4 {
		t.Fatalf("usage = %+v", response.Usage)
	}
}

func TestStreamReassemblesSSEAndUsage(t *testing.T) {
	t.Parallel()
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"ok\\\":\"}}],\"usage\":{\"prompt_tokens\":2}}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"true}\"}}],\"usage\":{\"completion_tokens\":3}}\n\ndata: [DONE]\n"
	response, err := providerOver(reply(http.StatusOK, eventStream, body)).Stream(t.Context(), modelRequest())
	if err != nil {
		t.Fatal(err)
	}
	if string(response.DecisionJSON) != `{"ok":true}` || response.Usage.InputTokens != 2 || response.Usage.OutputTokens != 3 || !response.UsageReported {
		t.Fatalf("response = %+v", response)
	}
}

func TestStreamRefusesWhatItCannotAccountFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider *native.OpenAICompatibleProvider
		want     string
	}{
		{"missing provider", nil, "endpoint and model are required"},
		{"missing endpoint", &native.OpenAICompatibleProvider{Model: "m"}, "endpoint and model are required"},
		{"missing model", &native.OpenAICompatibleProvider{Endpoint: "https://model.invalid"}, "endpoint and model are required"},
		{"client without a timeout", func() *native.OpenAICompatibleProvider {
			provider := providerOver(reply(http.StatusOK, "", ""))
			provider.Client = &http.Client{}
			return provider
		}(), "HTTP client timeout is required"},
		{"JSON without usage", providerOver(reply(http.StatusOK, "", `{"choices":[{"message":{"content":"{}"}}]}`)), "response omitted usage"},
		{"stream without usage", providerOver(reply(http.StatusOK, eventStream, "data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n")), "stream omitted usage"},
		{"response without choices", providerOver(reply(http.StatusOK, "", `{"choices":[],"usage":{}}`)), "contains no choices"},
		{"response that is not json", providerOver(reply(http.StatusOK, "", `not json`)), "decode model response"},
		{"stream event that is not json", providerOver(reply(http.StatusOK, eventStream, "data: not json\n")), "decode model stream event"},
		{"client error", providerOver(reply(http.StatusBadRequest, "", `bad schema`)), "400 Bad Request: bad schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.provider.Stream(t.Context(), modelRequest())
			var retryable *native.RetryableError
			if err == nil || !strings.Contains(err.Error(), tt.want) || errors.As(err, &retryable) {
				t.Fatalf("Stream() = %v, want a non-retryable error containing %q", err, tt.want)
			}
		})
	}
}

func TestStreamMarksOverloadAndServerFailuresRetryable(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			_, err := providerOver(reply(status, "", "try later")).Stream(t.Context(), modelRequest())
			var retryable *native.RetryableError
			if !errors.As(err, &retryable) || !strings.Contains(err.Error(), "try later") {
				t.Fatalf("Stream() = %v, want a retryable error carrying the provider's message", err)
			}
		})
	}
}

func TestStreamHonorsCancellationAndNamesItself(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	provider := providerOver(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	}))
	if _, err := provider.Stream(ctx, modelRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream() = %v, want context.Canceled", err)
	}
	if provider.Name() != "openai-compatible" {
		t.Fatalf("Name() = %q", provider.Name())
	}
}
