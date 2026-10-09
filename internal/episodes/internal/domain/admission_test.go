package domain

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestAdmissionRecordSurvivesTheRequestRoundTrip(t *testing.T) {
	t.Parallel()
	digest := func(seed byte) []byte { return append(make([]byte, 31), seed) }
	want := episodeledger.Admission{
		EpisodeID: "epi", SchedulerItemID: "item", TenantID: "tenant", SituationID: "sit", SituationVersion: 3,
		ExecutorName: "exec", ExecutorVersion: "v2", ModelPolicy: "policy", PromptVersion: "p1",
		SnapshotSHA256: digest(1), PromptSHA256: digest(2), ObjectiveSHA256: digest(3), AdmissionKey: digest(4),
		RequestJSON: []byte(`{"k":1}`), DispatchPolicy: "active", PolicyEpoch: "epoch",
	}
	persisted := reflect.ValueOf(want)
	for i := range persisted.NumField() {
		if name := persisted.Type().Field(i).Name; name != "Kind" && persisted.Field(i).IsZero() {
			t.Fatalf("fixture leaves admission field %s zero; the round trip would not cover it", name)
		}
	}
	req := AdmittedRequest(want)
	digests, err := DecodeRequestDigests(&req)
	if err != nil {
		t.Fatal(err)
	}
	if got := AdmittedEpisode(&req, digests); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestRebindPreservesAdmissionEvidence(t *testing.T) {
	t.Parallel()
	compiled, item, inputs := assemblyFixture(t)
	inputs.Reconsideration = map[string]any{"correction_version": 2}
	compiled.Cognition.Executor.RiskCeiling = ""
	req, err := AssembleRequest(compiled, "epi", item, inputs)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validSnapshotDocument(t, func(doc map[string]any) { doc["situation_version"] = 3 })
	live, err := ValidateSnapshotEvidence(snapshot, persistedDigestOf(t, snapshot), "", "", "s1", 3, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := RebindRequest(req, 3, live)
	if err != nil {
		t.Fatal(err)
	}
	if req.SituationVersion != 2 || fresh.SituationVersion != 3 || fresh.SnapshotSHA256 != live.Digest || fresh.PromptSHA256 != req.PromptSHA256 || !bytes.Equal(fresh.AdmissionKey, req.AdmissionKey) {
		t.Fatal("rebind changed admission or missed snapshot")
	}
	var before, after map[string]any
	if err := json.Unmarshal(req.RequestJSON, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fresh.RequestJSON, &after); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"snapshot", "snapshot_digest", "situation_version"} {
		delete(before, key)
		delete(after, key)
	}
	a, _ := canonicaljson.Marshal(before)
	b, _ := canonicaljson.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("rebind changed original trigger, budget or correction evidence")
	}
}

func TestCostBudgetPreservesAdmissionCeiling(t *testing.T) {
	t.Parallel()
	if cost, err := CostBudget([]byte(`{"budget":{"cost_microunits":42}}`)); err != nil || cost != 42 {
		t.Fatalf("ceiling=%d err=%v", cost, err)
	}
	if _, err := CostBudget([]byte("{")); err == nil || !strings.HasPrefix(err.Error(), "decode episode cost budget:") {
		t.Fatalf("invalid cost document=%v", err)
	}
}

func TestAdmittedEpisodeDeclaresShadowWhenRequestHasNoPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, policy, want string }{
		{name: "unset", policy: "", want: spec.DispatchShadow},
		{name: "shadow", policy: spec.DispatchShadow, want: spec.DispatchShadow},
		{name: "active", policy: spec.DispatchActive, want: spec.DispatchActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			admission := AdmittedEpisode(&Request{DispatchPolicy: tt.policy}, RequestDigests{})
			if admission.DispatchPolicy != tt.want {
				t.Fatalf("admitted policy = %q, want %q", admission.DispatchPolicy, tt.want)
			}
		})
	}
}

func TestDecodeRequestDigestsNamesTheFirstUndecodableDigest(t *testing.T) {
	t.Parallel()
	good := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name                        string
		snapshot, prompt, objective string
		want                        string
	}{
		{"all decodable", good, good, good, ""},
		{"snapshot first", "bad", "bad", "bad", "decode snapshot digest"},
		{"prompt second", good, "bad", "bad", "decode prompt digest"},
		{"objective last", good, good, "bad", "decode objective digest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeRequestDigests(&Request{SnapshotSHA256: tc.snapshot, PromptSHA256: tc.prompt, ObjectiveSHA256: tc.objective})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("decodable digests refused: %v", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want prefix %q", err, tc.want)
			}
		})
	}
}

func TestRebindRefusesAnEntityChangeBeforeItDecodesTheRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		live       string
		wantPrefix string
	}{
		{"entity changed, request also malformed", "different", "live snapshot entity"},
		{"same entity, malformed request", "original", "decode bound episode request:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &Request{EntityID: "original", RequestJSON: []byte("{")}
			if _, err := RebindRequest(req, 2, &SnapshotEvidence{EntityID: tc.live}); err == nil || !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Fatalf("error = %v, want prefix %q", err, tc.wantPrefix)
			}
		})
	}
}

func TestBindAttemptIdentityAddsOnlyTheAttemptAndKeepsTheDocumentCanonical(t *testing.T) {
	t.Parallel()
	bound, err := BindAttemptIdentity([]byte(`{"b": 2, "a": 1}`), "att-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":1,"attempt_id":"att-1","b":2,"fence":3}`; string(bound) != want {
		t.Fatalf("bound = %s, want %s", bound, want)
	}
	if _, err := BindAttemptIdentity([]byte("{"), "att-1", 3); err == nil || !strings.HasPrefix(err.Error(), "decode request json:") {
		t.Fatalf("error = %v, want decode request json", err)
	}
}
