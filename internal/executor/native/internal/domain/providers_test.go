package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

func TestDeterministicProviderPlaysItsScriptThenDecidesNothing(t *testing.T) {
	t.Parallel()
	provider := &DeterministicProvider{Responses: []ModelResponse{{FinishReason: "scripted"}}}
	req := ModelRequest{Episode: &episodes.Request{EpisodeID: "epi", AttemptID: "att", SnapshotSHA256: "sha256:x"}, Prompt: "p", Objective: "o"}
	first, err := provider.Stream(t.Context(), req)
	if err != nil || first.FinishReason != "scripted" {
		t.Fatalf("scripted response = %+v, %v", first, err)
	}
	second, err := provider.Stream(t.Context(), req)
	if err != nil || len(second.DecisionJSON) == 0 || !second.UsageReported {
		t.Fatalf("default response = %+v, %v", second, err)
	}
	if _, err := ValidateDecision(req.Episode, second.DecisionJSON, nil); err != nil {
		t.Fatalf("the default decision must be valid for the request: %v", err)
	}
	if provider.Name() != "deterministic" {
		t.Fatalf("Name() = %q", provider.Name())
	}
}

func TestRetryableErrorUnwrapsItsCause(t *testing.T) {
	t.Parallel()
	root := errors.New("root")
	err := &RetryableError{Err: root}
	if !errors.Is(err, root) || !strings.Contains(err.Error(), "retryable provider error: root") {
		t.Fatalf("error = %v", err)
	}
}
