package domain

import (
	"testing"
	"time"
)

func TestACaseWithARecoveryCheckIsVerifiedOnlyWhenTheReadingReturnedToItsFrozenBaseline(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	acted := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	decision := map[string]any{"diagnosis_code": "bearing_wear", "intents": []any{map[string]any{"type": "schedule_maintenance",
		"parameters": map[string]any{"work_order": "bearing_wear"}}}}
	facts := func(reading, baseline float64) map[string]any {
		return map[string]any{"facts.vibration_mean_15m": reading, "facts.vibration_mean_24h": baseline}
	}
	c := rules.NewCase(CaseCandidate{Decision: decision, Acknowledged: true, ActedAt: acted, Facts: facts(4.2, 3.0)})
	closedAt, watermark := acted.Add(4*time.Hour), acted.Add(25*time.Hour)
	closing := func(snapshot map[string]any) CaseEvidence {
		return CaseEvidence{ClosedAt: &closedAt, Watermark: watermark, Closing: Snapshot{Facts: snapshot}}
	}
	cases := []struct {
		name     string
		evidence CaseEvidence
		verdict  string
		reason   string
	}{
		{"back within tolerance of the frozen baseline", closing(facts(3.2, 3.1)), CaseVerified, ReasonClosedAndSettled},
		{"the moving baseline absorbed the change", closing(facts(4.2, 4.1)), CaseNotVerified, ReasonNotRecovered},
		{"fell far below the frozen baseline", closing(facts(2.0, 3.0)), CaseNotVerified, ReasonNotRecovered},
		{"no closing version", CaseEvidence{ClosedAt: &closedAt, Watermark: watermark}, CaseNotVerified, ReasonNotRecovered},
	}
	for _, tc := range cases {
		if verdict, reason := rules.Verify(c, tc.evidence); verdict != tc.verdict || reason != tc.reason {
			t.Errorf("%s: %s (%s), want %s (%s)", tc.name, verdict, reason, tc.verdict, tc.reason)
		}
	}
}

func TestACaseWhoseCodeDeclaresNoRecoveryCheckKeepsTheSettleWindowRule(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	acted := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	closedAt, watermark := acted.Add(4*time.Hour), acted.Add(25*time.Hour)
	c := Case{Observed: Observed{DiagnosisCode: "transient_none"}, WindowEndsAt: acted.Add(24 * time.Hour), Features: map[string]any{"vibration_mean_24h": 3.0}}
	offLevel := CaseEvidence{ClosedAt: &closedAt, Watermark: watermark, Closing: Snapshot{Facts: map[string]any{"facts.vibration_mean_15m": 9.0}}}
	if verdict, reason := rules.Verify(c, offLevel); verdict != CaseVerified || reason != ReasonClosedAndSettled {
		t.Fatalf("a code without a recovery check = %s (%s), want verified by the settle window alone", verdict, reason)
	}
}

func TestEachMaintenanceDiagnosisOfThePumpStationDeclaresTheReadingItsFaultMoves(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	want := map[string]string{"bearing_wear": "vibration_mean_15m", "lubrication_due": "vibration_mean_15m", "sensor_drift": "vibration_mean_15m",
		"sensor_failure": "vibration_mean_15m", "misalignment": "axial_mean_15m", "overload": "current_mean_15m"}
	for code, reading := range want {
		if check, declared := rules.Spec().Promotion.RecoveryCheck(code); !declared || check.Reading != reading {
			t.Errorf("recovery check of %s = %+v (%v), want the %s reading", code, check, declared, reading)
		}
	}
}
