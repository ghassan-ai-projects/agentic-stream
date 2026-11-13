package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
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

func TestAWokenSituationPublishesAVersionWhoseWatchDeltaAdmitsTheTriggerAgain(t *testing.T) {
	t.Parallel()
	compiled := cognitionSpec()
	compiled.Cognition.Triggers[0].MaterialDelta = "delta.phase_changed || delta.watch_fired"
	rig := newRigWithCognition(t, compiled, true)
	appendEnvelope(t, rig.log, thingEnvelope("evt-1", "test.observed", 0, map[string]any{"level": 15.0}))
	runGlobal(t, rig.service)
	appendEnvelope(t, rig.log, thingEnvelope("evt-2", "test.observed", 1, map[string]any{"level": 16.0}))
	runGlobal(t, rig.service)
	if admitted := countRows(t, rig.db, "SELECT COUNT(*) FROM trigger_evaluations WHERE outcome = 'admitted'"); admitted != 1 {
		t.Fatalf("%d admitted evaluations before the wake, want 1", admitted)
	}
	situationID := queryText(t, rig.db, "SELECT situation_id FROM situations")
	if err := rig.service.WakeSituations(t.Context(), []string{situationID, "sit_not_held"}); err != nil {
		t.Fatal(err)
	}
	if versions := countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions"); versions != 2 {
		t.Fatalf("%d situation versions after the wake, want 2", versions)
	}
	if admitted := countRows(t, rig.db, "SELECT COUNT(*) FROM trigger_evaluations WHERE outcome = 'admitted'"); admitted != 2 {
		t.Fatalf("%d admitted evaluations after the wake, want 2: the woken version is a material delta", admitted)
	}
	if items := countRows(t, rig.db, "SELECT COUNT(*) FROM scheduler_items WHERE situation_version = 2"); items != 1 {
		t.Fatalf("%d scheduler items on the woken version, want 1", items)
	}
}

func TestAFailedReasoningWakesItsSituationOnceItsEventTimeBackoffHasPassed(t *testing.T) {
	t.Parallel()
	compiled := cognitionSpec()
	compiled.Cognition.Triggers[0].MaterialDelta = "delta.phase_changed || delta.reasoning_failed"
	compiled.Cognition.RetryFailedReasoning = &spec.RetryPolicy{After: "6m", Max: 2}
	rig := newRigWithCognition(t, compiled, true)
	appendEnvelope(t, rig.log, thingEnvelope("evt-1", "test.observed", 0, map[string]any{"level": 15.0}))
	runGlobal(t, rig.service)
	situationID := queryText(t, rig.db, "SELECT situation_id FROM situations")
	failed := []domain.FailedReasoning{{SituationID: situationID, SituationVersion: 1}}
	if err := rig.service.RetryFailedReasoning(t.Context(), failed); err != nil {
		t.Fatal(err)
	}
	if versions := countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions"); versions != 1 {
		t.Fatalf("%d situation versions before the backoff, want 1", versions)
	}
	appendEnvelope(t, rig.log, thingEnvelope("evt-2", "test.observed", 6*time.Minute, map[string]any{"level": 16.0}))
	runGlobal(t, rig.service)
	if err := rig.service.RetryFailedReasoning(t.Context(), failed); err != nil {
		t.Fatal(err)
	}
	if admitted := countRows(t, rig.db, "SELECT COUNT(*) FROM trigger_evaluations WHERE outcome = 'admitted' AND situation_version = 2"); admitted != 1 {
		t.Fatalf("%d admitted evaluations on the version woken after the backoff, want 1", admitted)
	}
}

func TestAWatchWakeIsNoMaterialChangeWhenNoTriggerThatNamesTheWatchApplies(t *testing.T) {
	t.Parallel()
	compiled := cognitionSpec()
	compiled.Cognition.Triggers = append(compiled.Cognition.Triggers, spec.Trigger{
		Name: "low", When: "features.level < 0", Score: "situation.severity", Threshold: 5, Lane: "fast", MaterialDelta: "delta.phase_changed || delta.watch_fired",
	})
	rig := newRigWithCognition(t, compiled, true)
	appendEnvelope(t, rig.log, thingEnvelope("evt-1", "test.observed", 0, map[string]any{"level": 15.0}))
	runGlobal(t, rig.service)
	situationID := queryText(t, rig.db, "SELECT situation_id FROM situations")
	if err := rig.service.WakeSituations(t.Context(), []string{situationID}); err != nil {
		t.Fatal(err)
	}
	if current := queryText(t, rig.db, "SELECT current_version || ' ' || last_material_version FROM situations"); current != "2 1" {
		t.Fatalf("current and last material version = %s, want 2 1: a pending approval of version 1 survives the wake", current)
	}
}
