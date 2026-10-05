package domain

import (
	"strings"
	"testing"
)

func TestReconsiderationRejectsPriorIdentityBeforeCommand(t *testing.T) {
	t.Parallel()
	row := ReconsiderationRow{PriorDecisionID: "expected", PriorDecisionJSON: []byte(`{"decision_id":"other"}`), CommandJSON: []byte(`invalid`)}
	_, err := row.ReconsiderationDocument(Evaluation{}, nil, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "decision_id identity mismatch:") {
		t.Fatalf("error precedence: %v", err)
	}
}

func TestReconsiderationCorrectionCopiesNestedEvidence(t *testing.T) {
	t.Parallel()
	nested := map[string]any{"reason": "old", "reading": 42}
	row := ReconsiderationRow{InvalidatedCommandID: "command", SupersededVersion: 2, CorrectionVersion: 3}
	correction := row.CorrectionDocument(Evaluation{TriggerName: "corrected"}, map[string]any{"correction": nested}, map[string]any{"reading": 0})
	if correction["reason"] != "corrected" || correction["reading"] != 42 || correction["superseded_version"] != 2 || correction["correction_version"] != 3 {
		t.Fatalf("correction: %#v", correction)
	}
	if nested["reason"] != "old" || len(nested) != 2 {
		t.Fatalf("mutated immutable evidence: %#v", nested)
	}
}
