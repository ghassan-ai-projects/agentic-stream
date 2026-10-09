package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

func shadowInputFixture() ShadowInput {
	return ShadowInput{
		TenantID: "default", EpisodeID: "epi_1", SituationID: "sit_1", SituationVersion: 1,
		AttemptID: "attempt_1", Fence: 1, SnapshotDigest: "sha256:" + strings.Repeat("1", 64),
		SnapshotJSON: []byte(`{"entity":{"id":"motor-1"},"phase":"warning"}`),
	}
}

func shadowOutputFixture(t *testing.T, input ShadowInput) ShadowOutput {
	t.Helper()
	decision := map[string]any{
		"decision_id": "dec_1", "episode_id": input.EpisodeID, "attempt_id": input.AttemptID, "fence": input.Fence,
		"snapshot_digest": input.SnapshotDigest, "situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"confidence": 0.5, "decision_type": "need_more_evidence", "intents": []any{},
	}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatal(err)
	}
	return ShadowOutput{ExecutorVersion: "tamoz-test", ManifestSHA256: "sha256:" + strings.Repeat("a", 64), DecisionJSON: raw, DecisionSHA256: digest}
}

func notifyCatalogRules(t *testing.T) ShadowRules {
	t.Helper()
	catalog, err := decisions.CompileIntentCatalog([]map[string]any{{"type": "notify", "risk_class": "R1", "parameter_schema": map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatal(err)
	}
	return ShadowRules{Catalog: catalog, AllowedTypes: map[string]struct{}{"notify": {}}, RiskCeiling: "R1"}
}

func TestValidateOutputAcceptsADecisionBoundToItsInput(t *testing.T) {
	t.Parallel()
	input := shadowInputFixture()
	validated, err := notifyCatalogRules(t).ValidateOutput(input, shadowOutputFixture(t, input), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Decision == nil || len(validated.DecisionSHA) != 32 || len(validated.ManifestSHA) != 32 || string(validated.Canonical) != string(validated.Output.DecisionJSON) {
		t.Fatalf("validated output = %+v", validated)
	}
}

func TestValidateOutputRejectsEachBindingFailureWithItsOwnReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*ShadowInput, *ShadowOutput)
		wantErr string
	}{
		{"missing executor version", func(_ *ShadowInput, o *ShadowOutput) { o.ExecutorVersion = "" }, "executor version is required"},
		{"decision JSON that is not JSON", func(_ *ShadowInput, o *ShadowOutput) { o.DecisionJSON = []byte("{") }, "decision JSON:"},
		{"decision JSON that is not canonical", func(_ *ShadowInput, o *ShadowOutput) {
			o.DecisionJSON = []byte(strings.Replace(string(o.DecisionJSON), "{", "{ ", 1))
		}, "decision JSON is not canonical"},
		{"malformed decision digest", func(_ *ShadowInput, o *ShadowOutput) { o.DecisionSHA256 = "sha256:short" }, "decision digest:"},
		{"decision digest of another document", func(_ *ShadowInput, o *ShadowOutput) { o.DecisionSHA256 = "sha256:" + strings.Repeat("0", 64) }, "decision digest does not match decision JSON"},
		{"snapshot that is not JSON", func(i *ShadowInput, _ *ShadowOutput) { i.SnapshotJSON = []byte("{") }, "decode shadow entity"},
		{"snapshot without an entity", func(i *ShadowInput, _ *ShadowOutput) { i.SnapshotJSON = []byte(`{"phase":"warning"}`) }, "entity id is required"},
		{"decision of another episode", func(i *ShadowInput, _ *ShadowOutput) { i.EpisodeID = "epi_other" }, "validate shadow decision:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := shadowInputFixture()
			output := shadowOutputFixture(t, input)
			tc.mutate(&input, &output)
			_, err := notifyCatalogRules(t).ValidateOutput(input, output, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateOutput = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestShadowInputCloneSharesNoBytesWithTheOriginal(t *testing.T) {
	t.Parallel()
	input := shadowInputFixture()
	input.Request.RequestJSON = []byte(`{"kind":"episode"}`)
	clone := input.Clone()
	clone.SnapshotJSON[0], clone.Request.RequestJSON[0] = ' ', ' '
	if input.SnapshotJSON[0] != '{' || input.Request.RequestJSON[0] != '{' {
		t.Fatalf("mutating the clone changed the original: snapshot=%q request=%q", input.SnapshotJSON, input.Request.RequestJSON)
	}
}
