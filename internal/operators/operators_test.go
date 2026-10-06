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
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := operators.NewOperatorRuntime("deployment", compiled, sources.Deterministic())
	if err != nil || runtime == nil {
		t.Fatalf("runtime = %v err %v", runtime, err)
	}
	state := operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{}}
	if len(state.OperatorStates) != 0 {
		t.Fatal("fresh partition state must be empty")
	}
	var feature operators.Feature
	var heartbeat operators.HeartbeatState
	if feature.Completeness != "" || heartbeat.BootID != "" || operators.CompletenessProvisional == "" {
		t.Fatal("zero values and the provisional completeness must be stable")
	}
}
