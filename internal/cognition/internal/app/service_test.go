package app

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func TestServiceRequiresASpecDeploymentAndTenant(t *testing.T) {
	t.Parallel()
	compiled := levelSpec(fastTrigger())
	tests := map[string]Config{
		"nothing":       {},
		"no spec":       {DeploymentID: "dep", TenantID: "tenant"},
		"no deployment": {TenantID: "tenant", Spec: compiled},
		"no tenant":     {DeploymentID: "dep", Spec: compiled},
	}
	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(config); err == nil || err.Error() != "cognition spec, deployment and tenant are required" {
				t.Fatalf("New error = %v, want the missing-configuration refusal", err)
			}
		})
	}
}

func TestServiceRefusesATriggerWhoseExpressionDoesNotCompile(t *testing.T) {
	t.Parallel()
	broken := fastTrigger()
	broken.When = "features.level >"
	_, err := New(Config{DeploymentID: "dep", TenantID: "tenant", Spec: levelSpec(broken)})
	if err == nil || !strings.Contains(err.Error(), "compile trigger programs") {
		t.Fatalf("New error = %v, want a trigger compile refusal", err)
	}
}

func TestProcessRequiresACallerTransaction(t *testing.T) {
	t.Parallel()
	service, err := New(Config{DeploymentID: "dep", TenantID: "tenant", Spec: levelSpec(fastTrigger())})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Process(t.Context(), store.Join(nil), situations.Version{SituationID: "sit-1", Version: 1})
	if err == nil || err.Error() != "cognition caller transaction is required" {
		t.Fatalf("Process error = %v, want the missing-transaction refusal", err)
	}
}

func TestTriggerEvaluationsAreReadBackWithTheirReasons(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	reader := store.NewReader(h.db.DB)
	listed, err := TriggerEvaluations(t.Context(), reader, testTenant, "sit-1", 1)
	if err != nil || len(listed) != 1 || listed[0].Outcome != "admitted" {
		t.Fatalf("TriggerEvaluations = %+v, %v", listed, err)
	}
	one, err := TriggerEvaluation(t.Context(), reader, testTenant, listed[0].TriggerID)
	if err != nil || one.TriggerID != listed[0].TriggerID || len(one.Reasons) != 1 {
		t.Fatalf("TriggerEvaluation = %+v, %v", one, err)
	}
	if _, err := TriggerEvaluation(t.Context(), reader, testTenant, "trg-unknown"); err == nil || !strings.Contains(err.Error(), "trigger evaluation trg-unknown") {
		t.Fatalf("an unknown evaluation: err = %v, want one naming it", err)
	}
}
