package situations_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func compileExample(t *testing.T) *spec.CompiledSpec {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestFacadeBuildsAnEngineForACompiledSpec(t *testing.T) {
	t.Parallel()
	engine, err := situations.NewEngine("deployment", "tenant", 0, compileExample(t), sources.Deterministic())
	if err != nil || engine == nil {
		t.Fatalf("engine = %v err %v", engine, err)
	}
}

func TestFacadeCELFeaturesDefaultsEveryOperatorOutput(t *testing.T) {
	t.Parallel()
	compiled := compileExample(t)
	features := situations.CELFeatures(compiled, map[string]any{}, nil)
	for _, operator := range compiled.Operators {
		if _, ok := features[operator.Output]; !ok {
			t.Errorf("features lack the output %q of operator %s", operator.Output, operator.Name)
		}
	}
}

func TestFacadeResolvedPhaseKeepsItsStoredText(t *testing.T) {
	t.Parallel()
	if situations.PhaseResolved != "resolved" {
		t.Fatalf("PhaseResolved = %q, want resolved: the text is stored in situation versions", situations.PhaseResolved)
	}
}
