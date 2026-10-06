package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
)

func TestProviderStreamKeepsToolOrderFragmentsAndUsage(t *testing.T) {
	t.Parallel()
	body := `: connected

data: {"choices":[{"delta":{"content":"  {","tool_calls":[{"index":2,"id":"two","function":{"name":"second","arguments":"{\"b\":"}}]}}],"usage":{"prompt_tokens":2}}

data: {"choices":[{"delta":{"content":"}","tool_calls":[{"index":0,"id":"zero","function":{"name":"first","arguments":"{}"}},{"index":2,"function":{"arguments":"2}"}}]}}],"usage":{"completion_tokens":3}}

data: {"usage":{"cost_microunits":4}}

data: [DONE]

data: malformed ignored after DONE
`
	response, err := parseSSE(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if string(response.DecisionJSON) != "{}" || len(response.ToolCalls) != 2 || response.ToolCalls[0].ID != "zero" || response.ToolCalls[1].ID != "two" || string(response.ToolCalls[1].Arguments) != `{"b":2}` || response.ToolCalls[1].Name != "second" {
		t.Fatalf("response=%+v", response)
	}
	if !response.UsageReported || response.Usage.InputTokens != 2 || response.Usage.OutputTokens != 3 || response.Usage.CostMicrounits != 4 {
		t.Fatalf("usage=%+v", response.Usage)
	}
}

func TestProviderStatusPreservesRetryClassificationAndBodyBound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code      int
		retryable bool
	}{{400, false}, {429, true}, {500, true}, {503, true}} {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			t.Parallel()
			response := &http.Response{StatusCode: tc.code, Status: http.StatusText(tc.code), Body: io.NopCloser(strings.NewReader(strings.Repeat("a", 16<<10) + "secret-tail"))}
			err := checkProviderStatus(response)
			var retryable *domain.RetryableError
			if err == nil || errors.As(err, &retryable) != tc.retryable || strings.Contains(err.Error(), "secret-tail") {
				t.Fatalf("status classification or bound changed: err=%v", err)
			}
		})
	}
}
