package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func TestWorkerDecisionDigestRefusesAmbiguousBytesAndMismatches(t *testing.T) {
	t.Parallel()
	document := map[string]any{"decision_id": "dec-1", "episode_id": "epi-1"}
	raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		sum  []byte
		want string
	}{
		{"bound", raw, sum, ""},
		{"ambiguous bytes", contractstest.AmbiguousKeyJSON(raw, "decision_id"), sum, "not valid JSON"},
		{"not json", []byte(`{`), sum, "not valid JSON"},
		{"wrong digest", raw, make([]byte, 32), "digest mismatch"},
		{"short digest", raw, sum[:31], "digest mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifyDecisionDigest(tc.raw, tc.sum)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
