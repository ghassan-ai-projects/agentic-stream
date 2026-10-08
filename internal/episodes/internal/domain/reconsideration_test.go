package domain

import (
	"strings"
	"testing"
)

func reconsiderationDelta() map[string]any {
	return map[string]any{
		"reconsideration_id": "rec", "superseded_version": 2.0, "correction_version": 3.0,
		"invalidated_command_id": "command", "invalidated_outcome_id": "outcome",
		"prior_decision": map[string]any{"decision_id": "dec"},
		"prior_command":  map[string]any{"command_id": "command"},
		"prior_outcome":  map[string]any{"outcome_id": "outcome"},
		"correction":     map[string]any{"reason": "old", "reading": 42},
	}
}

func TestTakeReconsiderationBuildsDocumentFromDeltaAndRemovesPriorEvidence(t *testing.T) {
	t.Parallel()
	delta := reconsiderationDelta()
	document, err := TakeReconsideration(SchedulerItem{SituationID: "sit"}, Evaluation{TriggerName: "corrected"}, delta, nil)
	if err != nil {
		t.Fatal(err)
	}
	if document["reconsideration_id"] != "rec" || document["situation_id"] != "sit" || document["invalidated_outcome_id"] != "outcome" {
		t.Fatalf("identity: %#v", document)
	}
	if outcomes, _ := document["outcomes"].([]any); len(outcomes) != 1 || outcomes[0].(map[string]any)["outcome_id"] != "outcome" {
		t.Fatalf("outcomes: %#v", document["outcomes"])
	}
	for _, key := range priorEvidenceKeys {
		if _, ok := delta[key]; ok {
			t.Fatalf("delta still carries %s", key)
		}
	}
}

func TestTakeReconsiderationRequiresEveryPriorDocument(t *testing.T) {
	t.Parallel()
	for _, key := range priorEvidenceKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			delta := reconsiderationDelta()
			delete(delta, key)
			_, err := TakeReconsideration(SchedulerItem{}, Evaluation{}, delta, nil)
			if err == nil || !strings.Contains(err.Error(), key+" is required") {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestReconsiderationCorrectionCopiesNestedEvidence(t *testing.T) {
	t.Parallel()
	delta := reconsiderationDelta()
	nested := delta["correction"].(map[string]any)
	correction := correctionDocument(Evaluation{TriggerName: "corrected"}, delta, map[string]any{"reading": 0})
	if correction["reason"] != "corrected" || correction["reading"] != 42 || correction["superseded_version"] != 2 || correction["correction_version"] != 3 {
		t.Fatalf("correction: %#v", correction)
	}
	if nested["reason"] != "old" || len(nested) != 2 {
		t.Fatalf("mutated immutable evidence: %#v", nested)
	}
}
