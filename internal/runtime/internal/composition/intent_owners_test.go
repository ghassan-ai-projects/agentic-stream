package composition_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	activeThermalSpecPath = "../../../../examples/real-world-sensor/zone-thermal-sim.situation.yaml"
	shadowThermalSpecPath = "../../../../examples/thermal-chamber/zone-thermal.situation.yaml"
)

func TestThePipelineStartsOnlyWhenEveryActiveIntentHasAnOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		specPath string
		effector actionport.RoutedEffector
		want     string
	}{
		{"an active spec under a fail-closed fallback names its first unowned intent", activeThermalSpecPath, device.NewFailClosedEffector(device.EffectProfilePhysical), `active-dispatch spec declares an intent no effector owns: route "set_indicator" has no effector`},
		{"an active spec under the simulated fallback starts", activeThermalSpecPath, device.NewSimulatedEffector(), ""},
		{"a shadow spec never dispatches, so it starts under a fail-closed fallback", shadowThermalSpecPath, device.NewFailClosedEffector(device.EffectProfilePhysical), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := spec.CompileFile(t.Context(), tt.specPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = runtime.NewPipeline(t.Context(), runtime.PipelineConfig{
				DB: storagetest.OpenTemp(t), Spec: compiled, TenantID: "default", Clock: sources.Physical(),
				IDGenerator: sources.Deterministic(), Executor: fixture.New(), Effector: tt.effector,
			})
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("NewPipeline = %v, want %q", err, tt.want)
			}
		})
	}
}
