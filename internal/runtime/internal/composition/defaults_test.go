package composition

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestPipelineDefaultsFillOnlyWhatIsUnset(t *testing.T) {
	t.Parallel()
	t.Run("an empty config gets the contract tenant, physical time, random ids and the simulated fixture planes", func(t *testing.T) {
		t.Parallel()
		got := pipelineDefaults(PipelineConfig{})
		if got.TenantID != contractsv1.TenantID {
			t.Errorf("tenant = %q, want %q", got.TenantID, contractsv1.TenantID)
		}
		if got.Clock == nil || got.IDGenerator == nil || got.Executor == nil || got.Effector == nil {
			t.Fatalf("defaults left a dependency unset: %+v", got)
		}
	})
	t.Run("without a decision clock scheduling runs on the wall clock", func(t *testing.T) {
		t.Parallel()
		clock := sources.NewVirtual(sources.Physical().Now())
		if got := pipelineDefaults(PipelineConfig{Clock: clock}); got.DecisionClock != sources.Clock(clock) {
			t.Fatalf("decision clock = %v, want the configured wall clock", got.DecisionClock)
		}
		if got := pipelineDefaults(PipelineConfig{}); got.DecisionClock == nil || got.DecisionClock != got.Clock {
			t.Fatalf("decision clock = %v, want the default physical clock %v", got.DecisionClock, got.Clock)
		}
	})
	t.Run("a configured decision clock is kept beside the wall clock", func(t *testing.T) {
		t.Parallel()
		wall, decision := sources.NewVirtual(sources.Physical().Now()), sources.NewVirtual(sources.Physical().Now())
		got := pipelineDefaults(PipelineConfig{Clock: wall, DecisionClock: decision})
		if got.Clock != sources.Clock(wall) || got.DecisionClock != sources.Clock(decision) {
			t.Fatalf("clocks = %v and %v, want the wall and the decision clock configured", got.Clock, got.DecisionClock)
		}
	})
	t.Run("configured values are kept", func(t *testing.T) {
		t.Parallel()
		clock, ids, executor, effector := sources.NewVirtual(sources.Physical().Now()), sources.Deterministic(), fixture.New(), device.NewSimulatedEffector()
		got := pipelineDefaults(PipelineConfig{TenantID: "tenant", Clock: clock, IDGenerator: ids, Executor: executor, Effector: effector})
		if got.TenantID != "tenant" || got.Clock != sources.Clock(clock) || got.IDGenerator != ids || got.Executor != executor || got.Effector != effector {
			t.Fatalf("defaults replaced a configured value: %+v", got)
		}
	})
}
