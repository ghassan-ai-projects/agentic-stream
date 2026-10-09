package domain

import (
	"bytes"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestValidateReturnsDecisionAndIntentProvenance(t *testing.T) {
	t.Parallel()
	document := validDecision()
	raw, digest := encodeDecision(t, document)

	result, err := Validate(raw, digest, validInput(t))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	intent := firstIntent(document)
	wantCanonical, err := canonicaljson.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	got := result.Intents[0]
	if result.DecisionID != "dec-1" || result.DecisionDigest != digest || len(result.Intents) != 1 {
		t.Fatalf("result = decision %q digest %q with %d intents", result.DecisionID, result.DecisionDigest, len(result.Intents))
	}
	if got.ID != "int-1" || got.Digest != wantDigest || !bytes.Equal(got.CanonicalJSON, wantCanonical) {
		t.Fatalf("intent provenance = %+v, want digest %s over %s", got, wantDigest, wantCanonical)
	}
}

func TestValidateCarriesCatalogPolicyIntoTheValidatedIntent(t *testing.T) {
	t.Parallel()
	result, err := validateDocument(t, validDecision(), validInput(t))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	got := result.Intents[0]
	if got.RiskClass != "R1" || got.RateLimitPerHour != 6 || !got.RequiresApproval {
		t.Fatalf("intent = risk %s rate %d approval %v, want R1 6 true", got.RiskClass, got.RateLimitPerHour, got.RequiresApproval)
	}
	if want := validationTime.Add(time.Hour); !got.ExpiresAt.Equal(want) {
		t.Fatalf("expires at %v, want %v", got.ExpiresAt, want)
	}
}

func TestValidateAbstainsOnlyWhenTheDecisionAsksForMoreEvidence(t *testing.T) {
	t.Parallel()
	t.Run("explicit abstention yields no intents", func(t *testing.T) {
		t.Parallel()
		document := validDecision()
		document["decision_type"] = "need_more_evidence"
		document["intents"] = []any{}
		result, err := validateDocument(t, document, validInput(t))
		if err != nil || len(result.Intents) != 0 {
			t.Fatalf("result = %+v err = %v, want an accepted abstention without intents", result, err)
		}
	})
	t.Run("empty intents without a decision type are refused", func(t *testing.T) {
		t.Parallel()
		document := validDecision()
		document["intents"] = []any{}
		_, err := validateDocument(t, document, validInput(t))
		requireRejection(t, err, "schema_invalid", "decision_schema")
	})
}

func TestParseDecisionNamesTheStoredDocumentFailure(t *testing.T) {
	t.Parallel()
	raw, digest := encodeDecision(t, validDecision())
	tests := []struct {
		name   string
		raw    []byte
		digest string
		field  string
	}{
		{"ambiguous bytes", contractstest.AmbiguousKeyJSON(raw, "decision_id"), digest, "canonical_json"},
		{"schema violation", []byte(`{}`), digest, "decision_schema"},
		{"wrong digest", raw, digestText("0"), "decision_digest"},
		{"missing digest", raw, "", "decision_digest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseDecision(test.raw, test.digest)
			requireRejection(t, err, "schema_invalid", test.field)
		})
	}
	t.Run("bound document", func(t *testing.T) {
		t.Parallel()
		if _, err := parseDecision(raw, digest); err != nil {
			t.Fatalf("parseDecision: %v", err)
		}
	})
}

func TestValidateRefusesUnknownDecisionProperties(t *testing.T) {
	t.Parallel()
	document := validDecision()
	document["unexpected"] = true
	_, err := validateDocument(t, document, validInput(t))
	requireRejection(t, err, "schema_invalid", "decision_schema")
}

func TestValidateRequiresTrustedInput(t *testing.T) {
	t.Parallel()
	t.Run("clock", func(t *testing.T) {
		t.Parallel()
		input := validInput(t)
		input.Now = time.Time{}
		_, err := validateDocument(t, validDecision(), input)
		requireRejection(t, err, "schema_invalid", "clock")
	})
	t.Run("compiled catalog", func(t *testing.T) {
		t.Parallel()
		input := validInput(t)
		input.IntentCatalog = nil
		_, err := validateDocument(t, validDecision(), input)
		requireRejection(t, err, "catalog_missing", "catalog")
	})
}

func TestValidateRefusesDecisionsBoundToAnotherDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(map[string]any)
		reason string
		field  string
	}{
		{"episode", func(d map[string]any) { d["episode_id"] = "epi-other" }, "snapshot_mismatch", "episode_id"},
		{"attempt", func(d map[string]any) { d["attempt_id"] = "att-other" }, "stale_attempt", "attempt_id"},
		{"fence", func(d map[string]any) { d["fence"] = 99 }, "stale_attempt", "fence"},
		{"snapshot digest", func(d map[string]any) { d["snapshot_digest"] = digestText("1") }, "snapshot_mismatch", "snapshot_digest"},
		{"situation", func(d map[string]any) { d["situation_id"] = "sit-other" }, "snapshot_mismatch", "situation_id"},
		{"situation version", func(d map[string]any) { d["situation_version"] = 3 }, "snapshot_mismatch", "situation_version"},
		{"situation omitted", func(d map[string]any) { delete(d, "situation_id") }, "snapshot_mismatch", "situation_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			test.mutate(document)
			_, err := validateDocument(t, document, validInput(t))
			requireRejection(t, err, test.reason, test.field)
		})
	}
}

func TestValidateDecisionValidityWindowEndsAtValidUntil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		validUntil string
		wantReject bool
	}{
		{"future", kernel.FormatTime(validationTime.Add(1)), false},
		{"exactly now", kernel.FormatTime(validationTime), true},
		{"past", kernel.FormatTime(validationTime.Add(-1)), true},
		{"offset time before now", "2026-08-12T11:30:00+02:00", true},
		{"offset time after now", "2026-08-12T10:30:00+00:00", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			document["valid_until"] = test.validUntil
			_, err := validateDocument(t, document, validInput(t))
			if !test.wantReject {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			requireRejection(t, err, "expired", "valid_until")
		})
	}
}

func TestValidationErrorMessageNamesReasonAndField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  *ValidationError
		want string
	}{
		{"with field", reject("expired", "valid_until", "gone"), "decision rejected: expired (valid_until)"},
		{"without details", &ValidationError{Reason: "expired"}, "decision rejected: expired"},
		{"details without field", &ValidationError{Reason: "expired", Details: map[string]any{"n": 1}}, "decision rejected: expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
}
