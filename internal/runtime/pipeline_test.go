package runtime_test

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"testing"
)

func TestPipelineFacadeRequiresConfiguredUseCases(t *testing.T) {
	t.Parallel()
	if _, err := runtime.NewPipeline(t.Context(), runtime.PipelineConfig{}); err == nil {
		t.Fatal("missing pipeline dependencies accepted")
	}
	var pipeline *runtime.Pipeline
	if err := pipeline.Start(t.Context()); err == nil {
		t.Fatal("nil pipeline started")
	}
	if err := pipeline.Close(); err != nil {
		t.Fatal(err)
	}
}
