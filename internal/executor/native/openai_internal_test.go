package native

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleProviderUsesBoundedDefaultClient(t *testing.T) {
	if defaultOpenAIHTTPClient == nil {
		t.Fatal("default HTTP client is nil")
	}
	if defaultOpenAIHTTPClient.Timeout <= 0 {
		t.Fatalf("default HTTP client timeout=%v, want positive timeout", defaultOpenAIHTTPClient.Timeout)
	}

	original := defaultOpenAIHTTPClient
	t.Cleanup(func() { defaultOpenAIHTTPClient = original })
	called := false
	defaultOpenAIHTTPClient = &http.Client{
		Timeout: time.Second,
		Transport: defaultRoundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":1}}`,
				)),
			}, nil
		}),
	}

	provider := &OpenAICompatibleProvider{Endpoint: "https://model.invalid/v1/chat/completions", Model: "test-model"}
	if _, err := provider.Stream(context.Background(), ModelRequest{DecisionSchema: []byte(`{"type":"object"}`)}); err != nil {
		t.Fatalf("default client request: %v", err)
	}
	if !called {
		t.Fatal("nil provider client did not use the bounded package default")
	}
}

type defaultRoundTripFunc func(*http.Request) (*http.Response, error)

func (f defaultRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
