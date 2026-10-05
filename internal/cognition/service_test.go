package cognition

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestConfiguredFacadeRefusesMissingTransaction(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("missing authority configuration accepted")
	}
	s, err := New(Config{DeploymentID: "dep", TenantID: "tenant", Spec: &spec.CompiledSpec{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Process(t.Context(), nil, situations.Version{}); err == nil {
		t.Fatal("missing caller transaction accepted")
	}
	if err := RecordCostRejectionReason(t.Context(), nil, "item", errors.New("budget")); err == nil {
		t.Fatal("missing cost refusal transaction accepted")
	}
}
