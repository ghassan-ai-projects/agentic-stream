package episodes_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestConstructionRejectsIncompleteExecution(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	compiled, err := spec.CompileFile(t.Context(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	epochs := &control.EpochControl{DB: db}
	tests := []struct {
		name string
		cfg  episodes.Config
	}{
		{"missing spec", episodes.Config{}},
		{"missing database", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{Executor: declinedExecutor{}, DecisionEpoch: epochs.AssertDecisionTx}}},
		{"missing executor", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, DecisionEpoch: epochs.AssertDecisionTx}}},
		{"missing epoch check", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}}}},
		{"owner epoch without a runtime owner check", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, OwnerEpoch: "epoch", DecisionEpoch: epochs.AssertDecisionTx}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if service, err := episodes.New(test.cfg); err == nil || service != nil {
				t.Fatalf("construction=(%v,%v)", service, err)
			}
		})
	}
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance"}
	if service, err := episodes.New(episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, OwnerEpoch: "epoch", RuntimeOwner: owner.Assert, DecisionEpoch: epochs.AssertDecisionTx}}); err != nil || service == nil {
		t.Fatalf("owned execution refused: %v %v", service, err)
	}
	// Shadow persistence is the episode store's own table, so a shadow spec needs
	// nothing beyond the database every execution already requires.
	shadowSpec := *compiled
	shadowSpec.Cognition.Executor.DispatchPolicy = "shadow"
	if service, err := episodes.New(episodes.Config{Spec: &shadowSpec, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, DecisionEpoch: epochs.AssertDecisionTx}}); err != nil || service == nil {
		t.Fatalf("shadow execution refused: %v %v", service, err)
	}
}

func TestAssemblyOnlyServiceRefusesExecution(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	service, err := episodes.New(episodes.Config{Spec: compiled})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.RunOnce(t.Context(), "tenant"); err == nil || processed {
		t.Fatalf("assembly-only execution=(%v,%v)", processed, err)
	}
}
