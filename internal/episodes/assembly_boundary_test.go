package episodes

import (
	"strings"
	"testing"
)

func TestPersistedRequestValidatesTraceBeforeBudget(t *testing.T) {
	t.Parallel()
	req := &Request{RequestJSON: []byte(`{"traceparent":"invalid","budget":{"wall_time":"invalid"},"snapshot":{"entity":{"id":5}}}`)}
	err := hydratePersistedRequest(req)
	if err == nil || !strings.HasPrefix(err.Error(), "validate persisted request trace context:") {
		t.Fatalf("error precedence: %v", err)
	}
	if req.wallTimeValidated || req.EntityID != "" {
		t.Fatalf("request populated after invalid trace: %#v", req)
	}
}

func TestReconsiderationRejectsPriorIdentityBeforeCommand(t *testing.T) {
	t.Parallel()
	row := reconsiderationRow{PriorDecisionID: "expected", PriorDecisionJSON: []byte(`{"decision_id":"other"}`), CommandJSON: []byte(`invalid`)}
	_, err := row.reconsiderationDocument(evaluation{}, nil, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "decision_id identity mismatch:") {
		t.Fatalf("error precedence: %v", err)
	}
}

func TestReconsiderationCorrectionCopiesNestedEvidence(t *testing.T) {
	t.Parallel()
	nested := map[string]any{"reason": "old", "reading": 42}
	row := reconsiderationRow{InvalidatedCommandID: "command", SupersededVersion: 2, CorrectionVersion: 3}
	correction := row.correctionDocument(evaluation{TriggerName: "corrected"}, map[string]any{"correction": nested}, map[string]any{"reading": 0})
	if correction["reason"] != "corrected" || correction["reading"] != 42 || correction["superseded_version"] != 2 || correction["correction_version"] != 3 {
		t.Fatalf("correction: %#v", correction)
	}
	if nested["reason"] != "old" || len(nested) != 2 {
		t.Fatalf("mutated immutable evidence: %#v", nested)
	}
}
