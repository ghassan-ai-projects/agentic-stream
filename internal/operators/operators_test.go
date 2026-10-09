package operators_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestFacadeBuildsAnOperatorRuntimeForACompiledSpec(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "examples", "predictive-maintenance", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := operators.NewOperatorRuntime("deployment", compiled, sources.Deterministic())
	if err != nil || runtime == nil {
		t.Fatalf("runtime = %v err %v", runtime, err)
	}
}

func TestFacadeCompletenessConstantsKeepTheirStoredText(t *testing.T) {
	t.Parallel()
	for want, got := range map[string]string{
		"provisional": string(operators.CompletenessProvisional),
		"on_time":     string(operators.CompletenessOnTime),
		"uncertain":   string(operators.CompletenessUncertain),
	} {
		if got != want {
			t.Errorf("completeness = %q, want %q: the text is stored in snapshots", got, want)
		}
	}
}
