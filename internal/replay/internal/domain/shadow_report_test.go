package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func shadowResultFixture() Result {
	return Result{
		Mode: ModeShadow, EventsProcessed: 3, VersionCount: 2, VersionsHash: "hash",
		ShadowComparisons: []ShadowComparisonResult{
			{EpisodeKey: "e1", ComparisonSHA256: "c1", DecisionsEqual: true, ComparisonJSON: []byte(`{"a":1}`), BaselineDecisionJSON: []byte(`{"b":1}`), TamozDecisionJSON: []byte(`{"t":1}`)},
			{EpisodeKey: "e2", ComparisonSHA256: "c2", ComparisonJSON: []byte(`{}`), BaselineDecisionJSON: []byte(`{}`), TamozDecisionJSON: []byte(`{}`)},
		},
		Findings: []Finding{{Code: "decision_differs", Message: "e2"}},
	}
}

func TestShadowReportJSONContract(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(shadowResultFixture().ShadowReport())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mode":"shadow","events_processed":3,"situation_versions":2,"versions_hash":"hash","effects_allowed":false,` +
		`"comparisons":[{"episode_key":"e1","comparison_sha256":"c1","decisions_equal":true,"comparison":{"a":1},"baseline_decision":{"b":1},"candidate_decision":{"t":1}},` +
		`{"episode_key":"e2","comparison_sha256":"c2","decisions_equal":false,"comparison":{},"baseline_decision":{},"candidate_decision":{}}],` +
		`"findings":[{"code":"decision_differs","message":"e2"}]}`
	if string(encoded) != want {
		t.Fatalf("shadow report JSON =\n%s\nwant\n%s", encoded, want)
	}
}

func TestShadowReportListsAreNeverNull(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(Result{Mode: ModeShadow}.ShadowReport())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded); !strings.HasSuffix(got, `"comparisons":[],"findings":[]}`) {
		t.Fatalf("empty report = %s", got)
	}
}

func TestDifferingComparisonsCountsUnequalDecisions(t *testing.T) {
	t.Parallel()
	if got := shadowResultFixture().DifferingComparisons(); got != 1 {
		t.Fatalf("DifferingComparisons = %d, want 1", got)
	}
}
