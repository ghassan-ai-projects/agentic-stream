package domain

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

func TestStorageDecisionDigestPrefersContractDigest(t *testing.T) {
	t.Parallel()
	decision := map[string]any{"decision_id": "d1", "confidence": 0.5}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicaljson.DigestSum(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatal(err)
	}
	digest, hasContract := StorageDecisionDigest(raw)
	if !hasContract || string(digest) != string(want) {
		t.Fatalf("digest = %x hasContract = %v, want %x true", digest, hasContract, want)
	}
}

func TestStorageDecisionDigestFallsBackToTheRawHash(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{"not json": "not-json", "not an object": `[1]`} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			digest, hasContract := StorageDecisionDigest([]byte(raw))
			if want := canonicaljson.Sum([]byte(raw)); hasContract || string(digest) != string(want) {
				t.Fatalf("digest = %x hasContract = %v, want %x false", digest, hasContract, want)
			}
		})
	}
}

func TestDecisionIDFromJSONReadsTheDeclaredIdentity(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ raw, want string }{
		"declared":    {`{"decision_id":"d1"}`, "d1"},
		"undeclared":  {`{}`, ""},
		"undecodable": {`nope`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := DecisionIDFromJSON([]byte(tc.raw)); got != tc.want {
				t.Fatalf("decision id = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidationFailureJSONRecordsReasonAndRawHash(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"decision_id":"d1"}`)
	tests := []struct {
		name              string
		err               error
		hasContractDigest bool
		wantReason        string
		wantRawHash       bool
	}{
		{"untyped without contract digest", errors.New("bad shape"), false, "schema_invalid", true},
		{"untyped with contract digest", errors.New("bad shape"), true, "schema_invalid", false},
		{"typed without contract digest", &decisions.ValidationError{Reason: "stale_situation"}, false, "stale_situation", true},
		{"typed with contract digest", &decisions.ValidationError{Reason: "stale_situation"}, true, "stale_situation", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document, err := ValidationFailureJSON(tc.err, raw, tc.hasContractDigest)
			if err != nil {
				t.Fatal(err)
			}
			var details map[string]any
			if err := json.Unmarshal(document, &details); err != nil {
				t.Fatal(err)
			}
			_, hasRawHash := details["raw_sha256"]
			if details["reason"] != tc.wantReason || hasRawHash != tc.wantRawHash {
				t.Fatalf("failure document = %s, want reason %q and raw hash %v", document, tc.wantReason, tc.wantRawHash)
			}
		})
	}
}
