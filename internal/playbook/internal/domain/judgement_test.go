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
	shadow := NewShadowEvaluation("t", Admission{EpisodeID: "ep-9", SituationID: "sit-1", SituationVersion: 2, EventTime: now}, match, recommendation)
	if !shadow.EventTime.Equal(now) || !audit.EventTime.Equal(fire.EventTime) {
		t.Fatalf("shadow at %v, audit at %v; want the admission's and the fire's event time", shadow.EventTime, audit.EventTime)
	}
	circuit := DisagreementCircuit(shadow, Observed{}, now)
	if circuit.Cause != CauseAuditDisagreement || RateCapCircuit("t", match, 4, now).Cause != CauseRateCap {
		t.Fatalf("circuit = %+v", circuit)
	}
	event, err := CircuitOpenedEvent(circuit, "//agentic-stream/tenants/t")
	if err != nil || event.Validate() != nil || event.Type != "playbook.rule.circuit_opened" {
		t.Fatalf("event = %+v, %v", event, err)
	}
}

func TestAKindBornWithAnotherDigestOpensThePinnedRulesCircuits(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pinned := rules.compiled[0].rule.PinnedKind
	if changes := rules.KindChanges("t", map[string]string{pinned.Name: pinned.Digest, "fan": "sha256:other"}, now); len(changes) != 0 {
		t.Fatalf("changes = %+v, want none while the pinned digest is born", changes)
	}
	changes := rules.KindChanges("t", map[string]string{pinned.Name: "sha256:probe-b"}, now)
	if len(changes) != 2 || changes[0].Cause != CauseKindChanged || changes[0].Evidence["born_digest"] != "sha256:probe-b" {
		t.Fatalf("changes = %+v, want both pump rules switched off", changes)
	}
}

func TestACaseIsVerifiedOnlyWhenItClosedAndStayedClosedThroughItsWindow(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	acted := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	decision := map[string]any{"diagnosis_code": "bearing_wear", "intents": []any{map[string]any{"type": "schedule_maintenance",
		"parameters": map[string]any{"work_order": "bearing_wear"}}}}
	c := rules.NewCase(CaseCandidate{SituationID: "s1", Decision: decision, Acknowledged: true, ActedAt: acted, RuleID: "bearing-wear", RuleVersion: 1})
	if c.Verdict != CasePending || !c.WindowEndsAt.Equal(acted.Add(24*time.Hour)) || c.Preset != "bearing_wear" || !c.RuleHandled() {
		t.Fatalf("new case = %+v, want a pending bearing_wear case with a 24h window", c)
	}
	at := func(hours int) *time.Time { moment := acted.Add(time.Duration(hours) * time.Hour); return &moment }
	cases := []struct {
		name     string
		evidence CaseEvidence
		verdict  string
	}{
		{"closed and stayed closed", CaseEvidence{ClosedAt: at(4), Watermark: *at(25)}, CaseVerified},
		{"window still open", CaseEvidence{ClosedAt: at(4), Watermark: *at(10)}, CasePending},
		{"reopened inside the window", CaseEvidence{ClosedAt: at(4), ReopenedAt: at(20), Watermark: *at(21)}, CaseNotVerified},
		{"masked: never closed", CaseEvidence{Watermark: *at(25)}, CaseNotVerified},
		{"reopened after the window", CaseEvidence{ClosedAt: at(4), ReopenedAt: at(30), Watermark: *at(30)}, CaseVerified},
	}
	for _, tc := range cases {
		if verdict, _ := c.Verify(tc.evidence); verdict != tc.verdict {
			t.Errorf("%s: verdict = %s, want %s", tc.name, verdict, tc.verdict)
		}
	}
	unacknowledged := rules.NewCase(CaseCandidate{Decision: decision, ActedAt: acted})
	unknownCode := rules.NewCase(CaseCandidate{Decision: map[string]any{"diagnosis_code": "transient_none"}, Acknowledged: true, ActedAt: acted})
	if unacknowledged.Reason != ReasonNotAcknowledged || unknownCode.Reason != ReasonNoSettleWindow {
		t.Fatalf("unacknowledged = %+v, no window = %+v", unacknowledged, unknownCode)
	}
	if NoEffectCircuit(c, acted).Cause != CauseNoEffect {
		t.Fatal("an ineffective rule case opens a no_effect circuit")
	}
}
