package replay

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestShadowComparisonPreservesDifferenceOrderAndDigest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		decisionDiff, manifestDiff bool
		want                       []string
	}{
		{"equal", false, false, []string{}},
		{"manifest only", false, true, []string{"manifest"}},
		{"decision only", true, false, []string{"decision"}},
		{"both", true, true, []string{"decision", "manifest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			digest := "sha256:" + strings.Repeat("0", 64)
			input := ShadowInput{EpisodeKey: "s1/1/t1", EpisodeID: "e1", SituationID: "s1", SituationVersion: 1, TriggerID: "t1", SnapshotDigest: digest, SpecDigest: digest, PolicyDigest: digest}
			baseline := validatedShadowOutput{canonical: []byte(`{"decision":1}`), output: ShadowOutput{ExecutorVersion: "v1", ManifestSHA256: digest, DecisionSHA256: digest}}
			tamoz := baseline
			if tc.decisionDiff {
				tamoz.canonical = []byte(`{"decision":2}`)
			}
			if tc.manifestDiff {
				tamoz.output.ManifestSHA256 = "sha256:" + strings.Repeat("1", 64)
			}
			comparison, err := buildShadowComparison(input, baseline, tamoz, "tenant", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(comparison.record.ComparisonJSON, &document); err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(tc.want)
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, err := json.Marshal(document["differences"])
			if err != nil || !bytes.Equal(gotJSON, wantJSON) {
				t.Fatalf("differences=%s want=%s err=%v", gotJSON, wantJSON, err)
			}
			if comparison.result.DecisionsEqual == tc.decisionDiff {
				t.Fatalf("decision equality changed: %+v", comparison.result)
			}
			if !canonicaljson.Verify(canonicaljson.DomainShadowComparison, document, comparison.result.ComparisonSHA256) || comparison.record.ComparisonID != "cmp_"+hex.EncodeToString(comparison.record.ComparisonSHA256) {
				t.Fatal("comparison identity or digest changed")
			}
			again, err := buildShadowComparison(input, baseline, tamoz, "tenant", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
			if err != nil || !reflect.DeepEqual(comparison.result, again.result) {
				t.Fatalf("wall time changed comparison result: err=%v", err)
			}
		})
	}
}
