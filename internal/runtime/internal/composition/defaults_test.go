package composition

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestPipelineDefaultsTenantIsTheContractTenant(t *testing.T) {
	t.Parallel()
	if got := pipelineDefaults(PipelineConfig{}).TenantID; got != contractsv1.TenantID {
		t.Fatalf("default tenant = %q, want %q", got, contractsv1.TenantID)
	}
}
