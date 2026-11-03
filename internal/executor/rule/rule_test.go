package rule_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/rule"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var ruleNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func serviceIntent() spec.Intent {
	return spec.Intent{Type: "service", Risk: "R1", Policy: "automatic", ModelWritableFields: []string{"order"},
		ParameterSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"entity_id": map[string]any{"type": "string"}, "order": map[string]any{"type": "string"}}},
		Presets: map[string]map[string]any{"wear": {"order": "wear"}}}
}

func ruleRequest(t *testing.T, section map[string]any) (*episodes.Request, []map[string]any) {
	t.Helper()
	catalog, digest, err := episodes.CompileIntentCatalog([]spec.Intent{serviceIntent()})
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{"executor": map[string]any{"intent_catalog": catalog, "intent_catalog_sha256": digest}}
	if section != nil {
		document[rule.Section] = section
	}
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return &episodes.Request{EpisodeID: "ep-1", TenantID: "t", SituationID: "s-1", SituationVersion: 3, EntityID: "pump-1",
		ExecutorName: rule.ExecutorName, AttemptID: "att-1", Fence: 1, RequestJSON: raw,
		SnapshotSHA256: "sha256:" + strings.Repeat("0", 64)}, catalog
}

func wearRule() map[string]any {
	return map[string]any{"rule_id": "wear-rule", "rule_version": 2, "playbook_digest": "sha256:pb", "diagnosis_code": "wear",
		"intent": "service", "preset": "wear", "parameters": map[string]any{"order": "wear"}, "features": map[string]any{"level": 4.2}}
}

func TestARuleDecisionPassesTheDecisionValidator(t *testing.T) {
	t.Parallel()
	req, catalog := ruleRequest(t, wearRule())
	outcome, err := rule.New(nil, sources.NewVirtual(ruleNow)).Execute(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := decisions.CompileIntentCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	result, err := decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, decisions.Input{
		EpisodeID: "ep-1", AttemptID: "att-1", Fence: 1, TenantID: "t", SituationID: "s-1", SituationVersion: 3, EntityID: "pump-1",
		SnapshotDigest: req.SnapshotSHA256, AllowedIntentTypes: map[string]struct{}{"service": {}}, RiskCeiling: "R1",
		IntentCatalog: compiled, DiagnosisCodes: map[string]struct{}{"wear": {}}, Now: ruleNow})
	if err != nil || len(result.Intents) != 1 || result.Intents[0].Type != "service" {
		t.Fatalf("validate the rule decision = %+v, %v", result, err)
	}
	var decision map[string]any
	if err := json.Unmarshal(outcome.DecisionJSON, &decision); err != nil {
		t.Fatal(err)
	}
	if decision["diagnosis_code"] != "wear" || !strings.Contains(decision["summary"].(string), "rule wear-rule v2 of playbook sha256:pb") {
		t.Fatalf("decision = %v, want the rule's diagnosis and name", decision)
	}
}

func TestTheSameRuleRequestGivesTheSameDecision(t *testing.T) {
	t.Parallel()
	executor := rule.New(nil, sources.NewVirtual(ruleNow))
	first, _ := ruleRequest(t, wearRule())
	second, _ := ruleRequest(t, wearRule())
	a, errA := executor.Execute(t.Context(), first)
	b, errB := executor.Execute(t.Context(), second)
	if errA != nil || errB != nil || a.DecisionSHA256 != b.DecisionSHA256 {
		t.Fatalf("decisions %s and %s (%v, %v), want the same", a.DecisionSHA256, b.DecisionSHA256, errA, errB)
	}
}

type modelExecutor struct{ calls int }

func (m *modelExecutor) Execute(context.Context, *episodes.Request) (*episodes.Outcome, error) {
	m.calls++
	return &episodes.Outcome{Status: "produced"}, nil
}

func TestEveryOtherRequestGoesToTheModel(t *testing.T) {
	t.Parallel()
	model := &modelExecutor{}
	req, _ := ruleRequest(t, nil)
	req.ExecutorName = "tamoz"
	if _, err := rule.New(model, nil).Execute(t.Context(), req); err != nil || model.calls != 1 {
		t.Fatalf("model calls = %d (%v), want the model to answer", model.calls, err)
	}
}

func TestARuleRequestWithoutItsRuleOrIntentIsRefused(t *testing.T) {
	t.Parallel()
	executor := rule.New(nil, nil)
	missing, _ := ruleRequest(t, nil)
	if _, err := executor.Execute(t.Context(), missing); err == nil || !strings.Contains(err.Error(), "carries no rule") {
		t.Fatalf("a request without a rule = %v, want it refused", err)
	}
	unknown := wearRule()
	unknown["intent"] = "refuel"
	req, _ := ruleRequest(t, unknown)
	if _, err := executor.Execute(t.Context(), req); err == nil || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("a rule intent outside the catalog = %v, want it refused", err)
	}
}
