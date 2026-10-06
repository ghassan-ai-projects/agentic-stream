package remote_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
)

func TestExecutorWithoutClientRefusesToRun(t *testing.T) {
	t.Parallel()
	executor := remote.NewExecutor(nil, "worker", "instance", nil)
	if _, err := executor.Execute(t.Context(), &episodes.Request{}); err == nil {
		t.Fatal("executor without a worker client ran")
	}
	with := remote.NewExecutorWithEvidence(nil, "worker", "instance", nil, "", nil)
	if _, err := with.Execute(t.Context(), nil); err == nil {
		t.Fatal("executor without a worker client accepted a nil request")
	}
}
