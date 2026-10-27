package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestTheProposedEnvelopeIsTheSourceCasesRangeAndTheCapWidensIt(t *testing.T) {
	t.Parallel()
	cases := []map[string]any{
		{"vibration_mean_15m": 4.0, "current_mean_15m": 30.0, "load": 0.8},
		{"vibration_mean_15m": 4.6, "current_mean_15m": 30.0},
		{"vibration_mean_15m": 4.2, "current_mean_15m": 30.0},
	}
	envelope, limits := ProposedEnvelope(cases, []string{"vibration_mean_15m", "current_mean_15m", "load"}, 0.5)
	if envelope["vibration_mean_15m"] != (spec.Range{Min: 4.0, Max: 4.6}) || limits["vibration_mean_15m"].Min > 3.71 || limits["vibration_mean_15m"].Max < 4.89 {
		t.Fatalf("vibration envelope %+v cap %+v, want 4.0-4.6 widened by half its width", envelope["vibration_mean_15m"], limits["vibration_mean_15m"])
	}
	if limits["current_mean_15m"] != (spec.Range{Min: 15, Max: 45}) {
		t.Fatalf("a constant feature's cap = %+v, want the margin of its magnitude", limits["current_mean_15m"])
	}
	if _, covered := envelope["load"]; covered {
		t.Fatal("a feature missing from a source case got an envelope")
	}
}

func TestMostMatchesOutsideTheCapSwitchTheRuleOff(t *testing.T) {
	t.Parallel()
	cases := map[CapCounts]bool{{Matches: 3, OutsideCap: 3}: false, {Matches: 4, OutsideCap: 2}: false, {Matches: 4, OutsideCap: 3}: true}
	for counts, outside := range cases {
		if counts.MostlyOutside() != outside {
			t.Errorf("%+v mostly outside = %v, want %v", counts, counts.MostlyOutside(), outside)
		}
	}
	if circuit := OutOfCapCircuit("t", bearingWear, CapCounts{Matches: 4, OutsideCap: 3}, time.Now()); circuit.Cause != CauseOutOfCap {
		t.Fatalf("circuit = %+v", circuit)
	}
}

func TestAnActiveMatchOutsideItsEnvelopeIsReportedWithItsCap(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	inCap := rules.Route(pumpFeatures(rules, 5.5, 2.0, 30), State{})
	outsideCap := rules.Route(pumpFeatures(rules, 7.0, 2.0, 30), State{})
	if inCap.OutsideEnvelope == nil || !inCap.OutsideEnvelope.InCap || outsideCap.OutsideEnvelope == nil || outsideCap.OutsideEnvelope.InCap {
		t.Fatalf("in cap %+v, outside cap %+v; want both outside the envelope, one inside the cap", inCap, outsideCap)
	}
}
