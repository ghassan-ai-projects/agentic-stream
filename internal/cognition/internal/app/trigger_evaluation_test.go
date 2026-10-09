package app

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func withSeverity(v situations.Version, severity int) situations.Version {
	v.Severity = severity
	return v
}

func TestEachTriggerGateRecordsItsOutcomeAndReasonDurably(t *testing.T) {
	t.Parallel()
	phaseMaterial := func() spec.Trigger { tr := fastTrigger(); tr.MaterialDelta = "delta.phase_changed"; return tr }
	severityMaterial := func() spec.Trigger { tr := fastTrigger(); tr.MaterialDelta = "delta.severity_change == 10"; return tr }
	highThreshold := func() spec.Trigger { tr := fastTrigger(); tr.Threshold = 100; return tr }
	tests := []struct {
		name        string
		trigger     spec.Trigger
		versions    []situations.Version
		wantOutcome string
		wantReason  string
		wantItems   int
	}{
		{"condition false", fastTrigger(), []situations.Version{candidateVersion("sit-1", 1, 5)}, "ignored", "trigger condition false", 0},
		{"admitted into its lane", phaseMaterial(), []situations.Version{candidateVersion("sit-1", 1, 15)}, "admitted", "meets threshold", 1},
		{"score below threshold", highThreshold(), []situations.Version{candidateVersion("sit-1", 1, 15)}, "ignored", "below threshold", 0},
		{"material delta false", phaseMaterial(), []situations.Version{candidateVersion("sit-1", 1, 15), candidateVersion("sit-1", 2, 20)}, "ignored", "material delta false", 0},
		{"material delta read from the previous version", severityMaterial(), []situations.Version{candidateVersion("sit-1", 1, 15), withSeverity(candidateVersion("sit-1", 2, 20), 20)}, "admitted", "meets threshold", 1},
		{"no material delta declared", fastTrigger(), []situations.Version{candidateVersion("sit-1", 1, 15), candidateVersion("sit-1", 2, 20)}, "admitted", "meets threshold", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTriggerHarness(t, tc.trigger)
			for _, v := range tc.versions {
				h.process(v)
			}
			last := tc.versions[len(tc.versions)-1]
			record := h.evaluation(last)
			if record.Outcome != tc.wantOutcome || record.Lane != "fast" || len(record.Reasons) != 1 || !strings.Contains(record.Reasons[0], tc.wantReason) {
				t.Fatalf("evaluation = %+v, want %s with one reason containing %q", record, tc.wantOutcome, tc.wantReason)
			}
			if got := h.itemCount(last); got != tc.wantItems {
				t.Fatalf("scheduler items = %d, want %d", got, tc.wantItems)
			}
		})
	}
}

func TestEvaluationStoresThePolicyDigestAndTheDeltaItSaw(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	record := h.evaluation(v)
	if record.PolicySHA256 != testSpecDigest || !strings.Contains(string(record.Delta), `"phase_changed":true`) {
		t.Fatalf("policy %q delta %s, want the spec digest and a first-version delta", record.PolicySHA256, record.Delta)
	}
}

func TestReEvaluatingTheSameVersionUpsertsInsteadOfDuplicating(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	h.reprocess(v)
	if items, evaluations := h.itemCount(v), scalar[int](h, "SELECT COUNT(*) FROM trigger_evaluations"); items != 1 || evaluations != 1 {
		t.Fatalf("scheduler items = %d, evaluations = %d; want one of each", items, evaluations)
	}
}

func TestTriggerReadsAnAbsentFactAsItsDefault(t *testing.T) {
	t.Parallel()
	compiled := levelSpec(spec.Trigger{
		Name: "warning_needs_diagnosis", When: `situation.phase == "candidate" && !features.heartbeat_missing_5m`,
		Score: "double(situation.severity)", Threshold: 5, Lane: "deep",
	})
	compiled.Operators = []spec.Operator{{Name: "heartbeat_missing", Kind: "missing_heartbeat", Output: "heartbeat_missing_5m"}}
	compiled.Situation.Reducers = []spec.Reducer{{Field: "facts.heartbeat_missing_5m", Strategy: "latest_event_time", Input: "heartbeat_missing_5m"}}
	h := newHarness(t, compiled)
	v := candidateVersion("sit-1", 1, 0)
	v.Facts = map[string]any{"facts.heartbeat_missing_5m": nil}
	h.process(v)
	if record := h.evaluation(v); record.Outcome != "admitted" || record.Lane != "deep" {
		t.Fatalf("evaluation = %+v, want admitted into the deep lane", record)
	}
}

func TestProcessMarksTheVersionReasonedAndOnlyMaterialVersionsMaterial(t *testing.T) {
	t.Parallel()
	trigger := fastTrigger()
	trigger.MaterialDelta = "delta.phase_changed"
	h := newTriggerHarness(t, trigger)
	h.process(candidateVersion("sit-1", 1, 15))
	h.process(candidateVersion("sit-1", 2, 20))
	reasoned := scalar[int](h, "SELECT last_reasoned_version FROM situations WHERE situation_id = 'sit-1'")
	material := scalar[int](h, "SELECT last_material_version FROM situations WHERE situation_id = 'sit-1'")
	if reasoned != 2 || material != 1 {
		t.Fatalf("last reasoned = %d, last material = %d; want 2 and 1: version 2 changed nothing material", reasoned, material)
	}
}

func TestProcessNamesTheTriggerWhoseEvaluationFailed(t *testing.T) {
	t.Parallel()
	trigger := fastTrigger()
	trigger.MaterialDelta = "features.absent"
	h := newTriggerHarness(t, trigger)
	err := h.tryProcess(candidateVersion("sit-1", 1, 15))
	if err == nil || !strings.Contains(err.Error(), "evaluate trigger high:") {
		t.Fatalf("error = %v, want one naming trigger high", err)
	}
}
