package episodes_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestConstructionRejectsIncompleteExecution(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := predictiveMaintenanceSpec(t)
	epochs := &control.EpochControl{DB: db}
	tests := []struct {
		name string
		cfg  episodes.Config
		want string
	}{
		{"missing spec", episodes.Config{}, "episode spec is required"},
		{"missing database", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{Executor: declinedExecutor{}, DecisionEpoch: epochs.AssertDecisionTx}}, "database, executor and decision epoch check"},
		{"missing executor", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, DecisionEpoch: epochs.AssertDecisionTx}}, "database, executor and decision epoch check"},
		{"missing epoch check", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}}}, "database, executor and decision epoch check"},
		{"owner epoch without a runtime owner check", episodes.Config{Spec: compiled, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, OwnerEpoch: "epoch", DecisionEpoch: epochs.AssertDecisionTx}}, "runtime owner check is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, err := episodes.New(test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) || service != nil {
				t.Fatalf("New = (%v, %v), want no service and an error containing %q", service, err, test.want)
			}
		})
	}
}

func TestConstructionAcceptsOwnedAndShadowExecution(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	epochs := &control.EpochControl{DB: db}
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance"}
	shadowSpec := *predictiveMaintenanceSpec(t)
	shadowSpec.Cognition.Executor.DispatchPolicy = "shadow"
	tests := map[string]episodes.Config{
		"an owner epoch fenced by its runtime owner check": {Spec: predictiveMaintenanceSpec(t), Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, OwnerEpoch: "epoch", RuntimeOwner: owner.Assert, DecisionEpoch: epochs.AssertDecisionTx}},
		"a shadow spec with nothing beyond the database":   {Spec: &shadowSpec, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, DecisionEpoch: epochs.AssertDecisionTx}},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if service, err := episodes.New(cfg); err != nil || service == nil {
				t.Fatalf("New = (%v, %v), want a service", service, err)
			}
		})
	}
}

func TestAssemblyOnlyServiceRefusesExecution(t *testing.T) {
	t.Parallel()
	service, err := episodes.New(episodes.Config{Spec: predictiveMaintenanceSpec(t)})
	if err != nil {
		t.Fatal(err)
	}

	processed, err := service.RunOnce(t.Context(), "tenant")

	if err == nil || !strings.Contains(err.Error(), "assembly only") || processed {
		t.Fatalf("RunOnce = (%v, %v), want a refusal naming assembly-only configuration", processed, err)
	}
}
