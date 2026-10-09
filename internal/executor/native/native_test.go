package native_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

func TestNativeExecutorConformsToTheExecutorPort(t *testing.T) {
	t.Parallel()
	executor, err := native.New(native.Config{Provider: &native.DeterministicProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("produced outcome", func(t *testing.T) {
		t.Parallel()
		if err := executorconformance.Run(t.Context(), executor); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()
		if err := executorconformance.RunCanceled(t.Context(), executor); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFacadeBuildsTheScopedEvidenceTool(t *testing.T) {
	t.Parallel()
	tool := native.NewSQLiteEvidenceTool(nil, "evidence.get", "tenant", "entity", time.Time{})
	if tool.Name() != "evidence.get" {
		t.Fatalf("Name() = %q, want the configured tool name", tool.Name())
	}
}
