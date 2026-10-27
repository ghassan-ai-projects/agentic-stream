package domain

import (
	"testing"
	"time"
)

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
