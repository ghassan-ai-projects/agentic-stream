package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const ruleDiagnosisCatalog = `[{"code":"unknown","description":"no confident diagnosis"},{"code":"wear","description":"worn"}]`

func withRuleTarget(s *spec.CompiledSpec) {
	s.Cognition.Executor.DiagnosisCatalog = ruleDiagnosisCatalog
	s.Actions.Intents[0].Presets = map[string]map[string]any{"wear": {"reason": "wear"}}
	s.Actions.Intents[0].ModelWritableFields = []string{"reason"}
}

type ruleChange func(*spec.Rule)

func thingPlaybook(mode string, changes ...ruleChange) *spec.CompiledPlaybook {
	rule := spec.Rule{
		ID: "high-level", Version: 1, DiagnosisCode: "wear", When: "features.level > 10",
		Recommend: spec.Recommendation{Intent: "create_maintenance_ticket", Preset: "wear"},
		Envelope:  map[string]spec.Range{"level": {Min: 10, Max: 20}}, Cap: map[string]spec.Range{"level": {Min: 5, Max: 30}},
		Mode: mode, AuditEvery: 1000, RateCapPerHour: 10, Reads: []string{"level"},
	}
	for _, change := range changes {
		change(&rule)
	}
	return &spec.CompiledPlaybook{SchemaVersion: "agentic-stream/v1", Digest: "sha256:" + strings.Repeat("ab", 32), Rules: []spec.Rule{rule}}
}

func auditEveryCase(rule *spec.Rule) { rule.AuditEvery = 1 }

func rateCapOne(rule *spec.Rule) { rule.RateCapPerHour = 1 }

func TestAnActiveRuleAnswersItsCaseWithoutTheModel(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withRuleTarget), runtime.PipelineConfig{Playbook: thingPlaybook(spec.RuleModeActive)})
	if _, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatal(err)
	}
	if executor := scalar[string](t, db, "SELECT executor_name FROM episodes"); executor != "rule" {
		t.Fatalf("episode executor = %q, want the rule executor", executor)
	}
	decision := scalar[string](t, db, "SELECT CAST(raw_json AS TEXT) FROM decisions WHERE validation_status = 'accepted'")
	if !strings.Contains(decision, `"diagnosis_code":"wear"`) || !strings.Contains(decision, `"reason":"wear"`) || !strings.Contains(decision, "rule high-level v1") {
		t.Fatalf("decision = %s, want the rule's diagnosis, preset and name", decision)
	}
	if fires, status := countRows(t, db, "rule_fires"), scalar[string](t, db, "SELECT status FROM commands"); fires != 1 || status != "succeeded" {
		t.Fatalf("%d fires, command %q; want one fire whose command succeeded", fires, status)
	}
}

func TestAShadowRuleIsJudgedAgainstTheModelsDecision(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withRuleTarget), runtime.PipelineConfig{Playbook: thingPlaybook(spec.RuleModeShadow)})
	if _, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatal(err)
	}
	if executor := scalar[string](t, db, "SELECT executor_name FROM episodes"); executor == "rule" {
		t.Fatal("a shadow rule answered the case")
	}
	if verdict := scalar[string](t, db, "SELECT verdict FROM rule_evaluations WHERE kind = 'shadow'"); verdict != "disagree" {
		t.Fatalf("shadow verdict = %q, want disagree: the fixture model names no diagnosis", verdict)
	}
	if circuits := countRows(t, db, "rule_circuits"); circuits != 0 {
		t.Fatalf("%d circuits opened by a shadow disagreement, want none", circuits)
	}
}

func TestADisagreeingAuditOpensTheRulesCircuit(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withRuleTarget), runtime.PipelineConfig{Playbook: thingPlaybook(spec.RuleModeActive, auditEveryCase)})
	for _, trace := range []string{highLevelTrace(t), writeTrace(t, levelEvent("evt-2", "ent-2", 1))} {
		if _, err := pipeline.RunJSONL(t.Context(), trace); err != nil {
			t.Fatal(err)
		}
	}
	if audit := scalar[string](t, db, "SELECT dispatch_policy FROM episodes WHERE executor_name != 'rule'"); audit != "shadow" {
		t.Fatalf("audit episode dispatch = %q, want shadow", audit)
	}
	if cause := scalar[string](t, db, "SELECT cause FROM rule_circuits WHERE state = 'open'"); cause != "audit_disagreement" {
		t.Fatalf("circuit cause = %q, want audit_disagreement", cause)
	}
	if alerts := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_type = 'playbook.rule.circuit_opened'"); alerts != 1 {
		t.Fatalf("%d circuit notifications, want one", alerts)
	}
	if commands := countRows(t, db, "commands"); commands != 1 {
		t.Fatalf("%d commands, want only the rule's: an audit never dispatches", commands)
	}
}

func TestAnOpenCircuitSendsTheNextCaseToTheModel(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withRuleTarget), runtime.PipelineConfig{Playbook: thingPlaybook(spec.RuleModeActive, rateCapOne)})
	trace := writeTrace(t, levelEvent("evt-1", "ent-1", 15), levelEvent("evt-2", "ent-2", 15))
	if _, err := pipeline.RunJSONL(t.Context(), trace); err != nil {
		t.Fatal(err)
	}
	if fires := countRows(t, db, "rule_fires"); fires != 1 {
		t.Fatalf("%d fires, want one before the rate cap", fires)
	}
	if cause := scalar[string](t, db, "SELECT cause FROM rule_circuits WHERE state = 'open'"); cause != "rate_cap" {
		t.Fatalf("circuit cause = %q, want rate_cap", cause)
	}
	if model := scalar[int](t, db, "SELECT COUNT(*) FROM episodes WHERE executor_name != 'rule'"); model != 1 {
		t.Fatalf("%d model episodes, want the capped case to go to the model", model)
	}
}

func withSecondTrigger(s *spec.CompiledSpec) {
	second := s.Cognition.Triggers[0]
	second.Name = "also_high"
	s.Cognition.Triggers = append(s.Cognition.Triggers, second)
}

func TestASecondItemOfABusySituationWaitsForTheNextPass(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active", withSecondTrigger), runtime.PipelineConfig{})
	if _, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatalf("a second item for a situation with a live episode failed the batch: %v", err)
	}
	if episodes := countRows(t, db, "episodes"); episodes != 1 {
		t.Fatalf("%d episodes after the first pass, want one while the other item waits", episodes)
	}
	if _, err := pipeline.RunJSONL(t.Context(), writeTrace(t, levelEvent("evt-2", "ent-2", 1))); err != nil {
		t.Fatal(err)
	}
	if episodes := countRows(t, db, "episodes"); episodes != 2 {
		t.Fatalf("%d episodes after the second pass, want the waiting item admitted", episodes)
	}
}
