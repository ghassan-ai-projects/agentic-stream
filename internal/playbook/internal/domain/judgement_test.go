package domain

import (
	"testing"
	"time"
)

func TestAuditSampleIsDeterministicAndAboutOneInEvery(t *testing.T) {
	t.Parallel()
	due, sample := 0, make([]bool, 3001)
	for version := 1; version <= 3000; version++ {
		sample[version] = AuditDue("sit-1", version, 3)
	}
	for version := 1; version <= 3000; version++ {
		if AuditDue("sit-1", version, 3) != sample[version] {
			t.Fatal("the audit sample is not deterministic")
		}
		if sample[version] {
			due++
		}
	}
	if due < 850 || due > 1150 || !AuditDue("sit-1", 7, 1) {
		t.Fatalf("%d of 3000 versions due at one in three; every version due at one in one", due)
	}
}

func TestObserveNamesTheModelsDiagnosisIntentAndPreset(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	decision := map[string]any{"diagnosis_code": "bearing_wear", "intents": []any{
		map[string]any{"type": "install_watch_condition", "parameters": map[string]any{}},
		map[string]any{"type": "schedule_maintenance", "parameters": map[string]any{"work_order": "bearing_wear", "entity_id": "p-1"}},
	}}
	observed := rules.Observe(decision)
	if observed != (Observed{DiagnosisCode: "bearing_wear", Intent: "schedule_maintenance", Preset: "bearing_wear"}) {
		t.Fatalf("observed = %+v", observed)
	}
	routing := rules.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{})
	recommendation := rules.Recommend(*routing.Fire)
	if recommendation.Verdict(observed) != VerdictAgree {
		t.Fatal("the same diagnosis, intent and preset must agree")
	}
	if recommendation.Verdict(rules.Observe(map[string]any{"diagnosis_code": "bearing_wear"})) != VerdictDisagree {
		t.Fatal("a decision without an action must disagree with a rule that acts")
	}
}

func TestRecordsCarryTheirIdentityAndEvidence(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	match := *rules.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{}).Fire
	recommendation := rules.Recommend(match)
	fire := NewFire("t", "ep-9", "sit-1", 2, recommendation, 1, now)
	audit := NewAuditEvaluation(fire, "sch-1")
	request := AuditRequestFor(fire)
	if fire.FireID != "rfire_ep-9" || !fire.AuditDue || audit.FireID != fire.FireID || request.Key != AuditKey(fire) {
		t.Fatalf("fire %+v, audit %+v, request %+v", fire, audit, request)
	}
	shadow := NewShadowEvaluation("t", "ep-9", "sit-1", 2, match, recommendation)
	circuit := DisagreementCircuit(shadow, Observed{}, now)
	if circuit.Cause != CauseAuditDisagreement || RateCapCircuit("t", match, 4, now).Cause != CauseRateCap {
		t.Fatalf("circuit = %+v", circuit)
	}
	event, err := CircuitOpenedEvent(circuit, "//agentic-stream/tenants/t")
	if err != nil || event.Validate() != nil || event.Type != "playbook.rule.circuit_opened" {
		t.Fatalf("event = %+v, %v", event, err)
	}
}
