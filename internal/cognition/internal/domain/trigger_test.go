package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestTriggerEvaluationPreservesGateOrderAndPartialScore(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, when, score, material, outcome, reason string
		wantScore                                    float64
		wantError                                    bool
	}{
		{"condition first", "false", "features.absent", "true", "ignored", "trigger condition false", 0, false},
		{"material before threshold", "true", "3", "false", "ignored", "material delta false", 3, false},
		{"below threshold", "true", "3", "true", "ignored", "below threshold", 3, false},
		{"at threshold", "true", "5", "true", "admitted", "meets threshold", 5, false},
		{"material error keeps score", "true", "7", "features.absent", "ignored", "", 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			trigger := spec.Trigger{Name: "test", When: tc.when, Score: tc.score, MaterialDelta: tc.material, Threshold: 5, Lane: "fast"}
			compiled := &spec.CompiledSpec{Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}}}
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			engine, err := NewRules(compiled)
			if err != nil {
				t.Fatal(err)
			}
			eval, err := engine.Evaluate(EvaluationInput{Trigger: trigger, Current: situations.Version{SituationID: "s1", Version: 1}, Now: now, DeploymentID: "deployment"})
			if (err != nil) != tc.wantError || eval.Score != tc.wantScore || eval.Outcome != tc.outcome || !eval.EvaluatedAt.Equal(now) {
				t.Fatalf("evaluation=%+v err=%v", eval, err)
			}
			if tc.wantError {
				if len(eval.Reasons) != 0 {
					t.Fatalf("error added a gate reason: %v", eval.Reasons)
				}
			} else if len(eval.Reasons) != 1 || !strings.Contains(eval.Reasons[0], tc.reason) {
				t.Fatalf("gate reasons=%v", eval.Reasons)
			}
		})
	}
}

func newRules(t *testing.T, triggers ...spec.Trigger) *Rules {
	t.Helper()
	rules, err := NewRules(&spec.CompiledSpec{Digest: "sha256:" + strings.Repeat("a", 64), Cognition: spec.Cognition{Triggers: triggers}})
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func evaluate(t *testing.T, rules *Rules, trigger spec.Trigger, current situations.Version) Evaluation {
	t.Helper()
	eval, err := rules.Evaluate(EvaluationInput{Trigger: trigger, Current: current, Now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), DeploymentID: "deployment"})
	if err != nil {
		t.Fatal(err)
	}
	return eval
}

func TestRulesRefuseATriggerWhoseExpressionDoesNotCompile(t *testing.T) {
	t.Parallel()
	for field, trigger := range map[string]spec.Trigger{
		"when":          {Name: "t", When: "features.level >", Score: "1.0"},
		"score":         {Name: "t", When: "true", Score: "nothing()"},
		"materialDelta": {Name: "t", When: "true", Score: "1.0", MaterialDelta: "delta.facts_changed &&"},
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			_, err := NewRules(&spec.CompiledSpec{Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}}})
			if err == nil || !strings.Contains(err.Error(), "compile trigger programs: compile t:"+field) {
				t.Fatalf("error = %v, want one naming t:%s", err, field)
			}
		})
	}
}

func TestEvaluationRefusesAResultOfTheWrongType(t *testing.T) {
	t.Parallel()
	for name, trigger := range map[string]spec.Trigger{
		"condition that is not boolean": {Name: "t", When: "1.0 + 1.0", Score: "1.0", Threshold: 1},
		"score that is not a number":    {Name: "t", When: "true", Score: `"high"`, Threshold: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rules := newRules(t, trigger)
			_, err := rules.Evaluate(EvaluationInput{Trigger: trigger, Current: situations.Version{SituationID: "s", Version: 1}, Now: time.Now(), DeploymentID: "d"})
			if err == nil {
				t.Fatal("a result of the wrong type was accepted")
			}
		})
	}
}

func TestScoreMayBeAnIntegerOrADouble(t *testing.T) {
	t.Parallel()
	for score, want := range map[string]float64{"3": 3, "2.5": 2.5, "situation.severity": 60} {
		trigger := spec.Trigger{Name: "t", When: "true", Score: score, Threshold: 1}
		eval := evaluate(t, newRules(t, trigger), trigger, situations.Version{SituationID: "s", Version: 1, Severity: 60})
		if eval.Score != want || eval.Outcome != "admitted" {
			t.Errorf("score %s = %v (%s), want %v admitted", score, eval.Score, eval.Outcome, want)
		}
	}
}

func TestEvaluationRecordsWhatTheTriggerSaw(t *testing.T) {
	t.Parallel()
	trigger := spec.Trigger{Name: "hot", When: "true", Score: "5.0", Threshold: 5, Lane: "deep"}
	rules := newRules(t, trigger)
	current := situations.Version{SituationID: "s1", Version: 3}
	eval := evaluate(t, rules, trigger, current)
	if eval.TriggerName != "hot" || eval.SituationID != "s1" || eval.SituationVersion != 3 || eval.Lane != "deep" || eval.Threshold != 5 || !eval.Admitted() {
		t.Fatalf("evaluation = %+v", eval)
	}
	if eval.PolicySHA256 != "sha256:"+strings.Repeat("a", 64) || len(eval.DeltaJSON) == 0 || !strings.HasPrefix(eval.TriggerID, "trg_") {
		t.Fatalf("policy %q delta %q trigger id %q: want the spec digest, the delta and a trg_ identity", eval.PolicySHA256, eval.DeltaJSON, eval.TriggerID)
	}
	if again := evaluate(t, rules, trigger, current); again.TriggerID != eval.TriggerID {
		t.Fatalf("trigger id changed between identical evaluations: %s vs %s", again.TriggerID, eval.TriggerID)
	}
	next := current
	next.Version = 4
	if other := evaluate(t, rules, trigger, next); other.TriggerID == eval.TriggerID {
		t.Fatal("two versions of one Situation share a trigger evaluation id")
	}
}

func TestEvaluationTimeIsRecordedInUTC(t *testing.T) {
	t.Parallel()
	trigger := spec.Trigger{Name: "t", When: "false", Score: "1.0"}
	zone := time.FixedZone("UTC+5", 5*3600)
	eval, err := newRules(t, trigger).Evaluate(EvaluationInput{Trigger: trigger, Current: situations.Version{SituationID: "s", Version: 1}, Now: time.Date(2026, 1, 1, 5, 0, 0, 0, zone), DeploymentID: "d"})
	if err != nil || eval.EvaluatedAt.Location() != time.UTC || !eval.EvaluatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("evaluated at %v err %v, want 2026-01-01T00:00:00Z", eval.EvaluatedAt, err)
	}
}

func TestOnlyAnAdmittedEvaluationMayCreateQueueWork(t *testing.T) {
	t.Parallel()
	for outcome, want := range map[string]bool{"admitted": true, "ignored": false, "deferred": false, "rejected": false, "": false} {
		if got := (Evaluation{Outcome: outcome}).Admitted(); got != want {
			t.Errorf("Admitted() for %q = %v, want %v", outcome, got, want)
		}
	}
}
