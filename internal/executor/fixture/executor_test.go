package fixture_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/conformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
)

func TestFixtureExecutorConforms(t *testing.T) {
	t.Parallel()
	if err := conformance.Run(t.Context(), fixture.New()); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureDecisionPreservesIdentityAndDigest(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, raw, entity string }{
		{"bound snapshot", `{"snapshot":{"phase":"warning"},"trigger":{"trigger_name":"temperature"}}`, "motor"},
		{"missing snapshot", `{}`, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			req := &episodes.Request{EpisodeID: "episode", AttemptID: "attempt", Fence: 3, TenantID: "tenant", SituationID: "sit", SituationVersion: 2, SnapshotSHA256: "sha256:bound", EntityID: test.entity, RequestJSON: []byte(test.raw)}
			executor := fixture.New()
			outcome, err := executor.Execute(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			again, err := executor.Execute(t.Context(), req)
			if err != nil || !bytes.Equal(outcome.DecisionJSON, again.DecisionJSON) {
				t.Fatalf("determinism failed: %v", err)
			}
			if outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence || outcome.Status != "produced" {
				t.Fatalf("outcome=%#v", outcome)
			}
			var document map[string]any
			if err := json.Unmarshal(outcome.DecisionJSON, &document); err != nil {
				t.Fatal(err)
			}
			if !canonicaljson.Verify(canonicaljson.DomainDecision, document, outcome.DecisionSHA256) {
				t.Fatal("fixture decision digest changed")
			}
			if document["snapshot_digest"] != req.SnapshotSHA256 || document["decision_id"] != "dec_episode" {
				t.Fatal("fixture lost snapshot provenance or deterministic identity")
			}
		})
	}
}

func TestFixtureRejectsMalformedRequest(t *testing.T) {
	t.Parallel()
	if outcome, err := fixture.New().Execute(t.Context(), &episodes.Request{RequestJSON: []byte("{")}); err == nil || outcome != nil {
		t.Fatalf("malformed request=(%v,%v)", outcome, err)
	}
}
