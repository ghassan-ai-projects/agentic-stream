package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func assemblyFixture(t *testing.T) (*spec.CompiledSpec, SchedulerItem, AssemblyInputs) {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw := validSnapshotDocument(t, nil)
	evidence, err := ValidateSnapshotEvidence(raw, persistedDigestOf(t, raw), "", "", "s1", 2, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	item := SchedulerItem{SchedulerItemID: "sch", Kind: "standard", TriggerID: "trigger", TenantID: "tenant", SituationID: "s1", SituationVersion: 2}
	inputs := AssemblyInputs{Snapshot: evidence, Evaluation: Evaluation{TriggerID: "trigger", TriggerName: "warning", Score: 10, Threshold: 5, Lane: "fast"}, Delta: map[string]any{"change": 1}}
	return compiled, item, inputs
}

func TestRequestAssemblyBindsProvenanceAndEvidence(t *testing.T) {
	t.Parallel()
	compiled, item, inputs := assemblyFixture(t)
	req, err := AssembleRequest(compiled, "epi", item, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if req.EpisodeID != "epi" || req.SnapshotSHA256 != inputs.Snapshot.Digest || req.EntityID != "motor-1" {
		t.Fatalf("request = %#v", req)
	}
	if req.PromptSHA256 == "" || req.ObjectiveSHA256 == "" || len(req.AdmissionKey) != 32 {
		t.Fatal("unbound provenance")
	}
	var payload struct {
		SnapshotDigest string `json:"snapshot_digest"`
		Executor       struct {
			IntentCatalog []map[string]any `json:"intent_catalog"`
			Digest        string           `json:"intent_catalog_sha256"`
			Prompt        string           `json:"prompt_sha256"`
		} `json:"executor"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SnapshotDigest != req.SnapshotSHA256 || payload.Executor.Prompt != req.PromptSHA256 || !canonicaljson.Verify(canonicaljson.DomainIntentCatalog, payload.Executor.IntentCatalog, payload.Executor.Digest) {
		t.Fatal("request document disagrees with durable provenance")
	}
	digests, err := DecodeRequestDigests(req)
	if err != nil {
		t.Fatal(err)
	}
	admission := AdmittedEpisode(req, digests)
	if !bytes.Equal(admission.SnapshotSHA256, persistedDigestOf(t, inputs.Snapshot.JSON)) {
		t.Fatal("admission lost snapshot digest")
	}
	again, err := AssembleRequest(compiled, "epi", item, inputs)
	if err != nil || !bytes.Equal(req.RequestJSON, again.RequestJSON) {
		t.Fatalf("non-deterministic assembly: %v", err)
	}
}

func TestRequestAssemblyRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, reason string
		mutate       func(*spec.CompiledSpec)
	}{
		{"diagnosis JSON", "diagnosis catalog is not valid JSON", func(c *spec.CompiledSpec) { c.Cognition.Executor.DiagnosisCatalog = "{" }},
		{"empty intents", "intent catalog is empty", func(c *spec.CompiledSpec) { c.Actions.Intents = nil }},
		{"invalid budget", "validate episode budget", func(c *spec.CompiledSpec) { c.Cognition.Executor.Budget.WallTime = "soon" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			compiled, item, inputs := assemblyFixture(t)
			test.mutate(compiled)
			if _, err := AssembleRequest(compiled, "epi", item, inputs); err == nil || !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("error = %v, want %s", err, test.reason)
			}
		})
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
