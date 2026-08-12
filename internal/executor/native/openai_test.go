package native_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestOpenAICompatibleProviderDeclaresBoundedTools(t *testing.T) {
	var request map[string]any
	provider := &native.OpenAICompatibleProvider{
		Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, fmt.Errorf("read request: %w", err)
			}
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, fmt.Errorf("decode request: %w", err)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}]}`)), Header: make(http.Header)}, nil
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
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
