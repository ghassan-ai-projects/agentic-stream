package domain

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func TestParseDecisionReportsEachStoredDocumentFailureField(t *testing.T) {
	t.Parallel()
	document := validDecision()
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		raw    []byte
		digest string
		field  string
	}{
		{"bound", raw, digest, ""},
		{"ambiguous bytes", contractstest.AmbiguousKeyJSON(raw, "decision_id"), digest, "canonical_json"},
		{"schema", []byte(`{}`), digest, "decision_schema"},
		{"wrong digest", raw, "sha256:" + zeros(64), "decision_digest"},
		{"missing digest", raw, "", "decision_digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseDecision(tc.raw, tc.digest)
			var typed *ValidationError
			if tc.field == "" && err != nil || tc.field != "" && (!errors.As(err, &typed) || typed.Reason != "schema_invalid" || typed.Details["field"] != tc.field) {
				t.Fatalf("err = %v, want field %q", err, tc.field)
			}
		})
	}
}
