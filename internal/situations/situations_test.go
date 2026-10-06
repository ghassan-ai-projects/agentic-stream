package situations_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestFacadeBuildsAnEngineAndTheCELFeatureView(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := situations.NewEngine("deployment", "tenant", 0, compiled, sources.Deterministic())
	if err != nil || engine == nil {
		t.Fatalf("engine = %v err %v", engine, err)
	}
	features := situations.CELFeatures(compiled, map[string]any{}, nil)
	if features == nil {
		t.Fatal("feature view is nil")
	}
	var current situations.Situation
	var version situations.Version
	if current.Version != 0 || version.Version != 0 {
		t.Fatal("zero values must start at version zero")
	}
}
