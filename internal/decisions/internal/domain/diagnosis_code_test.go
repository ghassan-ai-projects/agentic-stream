package domain

import (
	"errors"
	"testing"
)

func TestValidateChecksTheDiagnosisCodeAgainstTheCatalog(t *testing.T) {
	t.Parallel()
	catalog := map[string]struct{}{"wear": {}, "drift": {}}
	cases := []struct {
		name     string
		code     string
		catalog  map[string]struct{}
		accepted bool
	}{
		{"cataloged code", "wear", catalog, true},
		{"uncataloged code", "rust", catalog, false},
		{"no catalog declared", "rust", nil, true},
		{"no code", "", catalog, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			if tc.code != "" {
				document["diagnosis_code"] = tc.code
			}
			input := validInput(t)
			input.DiagnosisCodes = tc.catalog
			raw, digest := encodeDecision(t, document)
			_, err := Validate(raw, digest, input)
			assertDiagnosisVerdict(t, err, tc.accepted)
		})
	}
}

func assertDiagnosisVerdict(t *testing.T, err error, accepted bool) {
	t.Helper()
	if accepted {
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
		return
	}
	var rejection *ValidationError
	if !errors.As(err, &rejection) || rejection.Details["field"] != "diagnosis_code" {
		t.Fatalf("Validate = %v, want a rejection of diagnosis_code", err)
	}
}
