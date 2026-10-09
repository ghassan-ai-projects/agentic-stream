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
	t.Run("configured values are kept", func(t *testing.T) {
		t.Parallel()
		clock, ids, executor, effector := sources.NewVirtual(sources.Physical().Now()), sources.Deterministic(), fixture.New(), device.NewSimulatedEffector()
		got := pipelineDefaults(PipelineConfig{TenantID: "tenant", Clock: clock, IDGenerator: ids, Executor: executor, Effector: effector})
		if got.TenantID != "tenant" || got.Clock != sources.Clock(clock) || got.IDGenerator != ids || got.Executor != executor || got.Effector != effector {
			t.Fatalf("defaults replaced a configured value: %+v", got)
		}
	})
}
