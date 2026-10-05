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
