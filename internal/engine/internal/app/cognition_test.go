package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func cognitionSpec() spec.CompiledSpec {
	compiled := restartSpec()
	compiled.Time.MaxOutOfOrderness = "2m"
	compiled.Inputs = []spec.Input{{Name: "level", EventType: "test.observed", EntityType: "thing"}}
	compiled.Situation.Phases = []spec.Phase{{Name: "candidate", Severity: 10}}
	compiled.Situation.Transitions = nil
	compiled.Cognition = spec.Cognition{Triggers: []spec.Trigger{{
		Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast", MaterialDelta: "delta.phase_changed",
	}}}
	return compiled
}

func TestANewSituationVersionReachesCognitionInTheSameTransactionWhenEnabled(t *testing.T) {
	t.Parallel()
	rig := newRigWithCognition(t, cognitionSpec(), true)
	appendEnvelope(t, rig.log, thingEnvelope("evt-1", "test.observed", 0, map[string]any{"level": 15.0}))
	runGlobal(t, rig.service)

	if outcome := queryText(t, rig.db, "SELECT outcome FROM trigger_evaluations WHERE tenant_id = 'default'"); outcome != "admitted" {
		t.Fatalf("trigger outcome = %q, want admitted", outcome)
	}
	if items := countRows(t, rig.db, "SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = 'default'"); items != 1 {
		t.Fatalf("scheduler items = %d, want 1", items)
	}
}

func TestCognitionIsNotReachedWhenDisabled(t *testing.T) {
	t.Parallel()
	rig := newRigWithCognition(t, cognitionSpec(), false)
	appendEnvelope(t, rig.log, thingEnvelope("evt-1", "test.observed", 0, map[string]any{"level": 15.0}))
	runGlobal(t, rig.service)

	if versions := countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions"); versions == 0 {
		t.Fatal("no Situation version was published, so the scenario proves nothing")
	}
	for _, table := range []string{"trigger_evaluations", "scheduler_items"} {
		if rows := countRows(t, rig.db, "SELECT COUNT(*) FROM "+table); rows != 0 {
			t.Fatalf("%s holds %d rows, want none with cognition disabled", table, rows)
		}
	}
}
