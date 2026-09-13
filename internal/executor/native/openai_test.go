package native_test

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

func TestOpenAICompatibleProviderDeclaresBoundedTools(t *testing.T) {
	var request map[string]any
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, fmt.Errorf("read request: %w", err)
			}
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, fmt.Errorf("decode request: %w", err)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)), Header: make(http.Header)}, nil
		})},
	}
	_, err := provider.Stream(context.Background(), native.ModelRequest{
		Prompt: "prompt", Objective: "objective", DecisionSchema: json.RawMessage(`{"type":"object"}`),
		Tools: []native.ToolDefinition{{Name: "evidence_get", Description: "read evidence", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := request["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("provider tools=%#v", request["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["function"].(map[string]any)["name"] != "evidence_get" {
		t.Fatalf("tool=%#v", tool)
	}
	streamOptions, ok := request["stream_options"].(map[string]any)
	if !ok || streamOptions["include_usage"] != true {
		t.Fatalf("stream_options=%#v, want include_usage=true", request["stream_options"])
	}
}

func TestOpenAICompatibleProviderReassemblesSSEAndUsage(t *testing.T) {
	first, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": `{"ok":`}}}, "usage": map[string]any{"prompt_tokens": 2}})
	second, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": "true}"}}}, "usage": map[string]any{"completion_tokens": 3}})
	body := "data: " + string(first) + "\n\ndata: " + string(second) + "\n\ndata: [DONE]\n"
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
	}
	response, err := provider.Stream(context.Background(), native.ModelRequest{DecisionSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(response.DecisionJSON) != `{"ok":true}` || response.Usage.InputTokens != 2 || response.Usage.OutputTokens != 3 {
		t.Fatalf("response=%+v", response)
	}
}

func TestOpenAICompatibleProviderHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			default:
				return nil, errors.New("request should have been canceled")
			}
		})},
	}
	if _, err := provider.Stream(ctx, native.ModelRequest{DecisionSchema: json.RawMessage(`{"type":"object"}`)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
}

func TestOpenAICompatibleProviderRejectsZeroTimeoutClient(t *testing.T) {
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model", Client: &http.Client{},
	}
	if _, err := provider.Stream(context.Background(), native.ModelRequest{DecisionSchema: json.RawMessage(`{"type":"object"}`)}); err == nil || !strings.Contains(err.Error(), "HTTP client timeout is required") {
		t.Fatalf("zero-timeout client error = %v", err)
	}
}

func TestOpenAICompatibleProviderRejectsJSONWithoutUsage(t *testing.T) {
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}]}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}
	if _, err := provider.Stream(context.Background(), native.ModelRequest{DecisionSchema: json.RawMessage(`{"type":"object"}`)}); err == nil || !strings.Contains(err.Error(), "response omitted usage") {
		t.Fatalf("missing JSON usage error=%v", err)
	}
}

func TestOpenAICompatibleProviderRejectsSSEWithoutUsage(t *testing.T) {
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			body := "data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n"
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
	}
	if _, err := provider.Stream(context.Background(), native.ModelRequest{DecisionSchema: json.RawMessage(`{"type":"object"}`)}); err == nil || !strings.Contains(err.Error(), "stream omitted usage") {
		t.Fatalf("missing SSE usage error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
