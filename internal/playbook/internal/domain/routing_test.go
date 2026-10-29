package domain

import "testing"

func TestRouteFiresTheOneActiveRuleInsideItsEnvelope(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	routing := rules.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{})
	if routing.Fire == nil || routing.Fire.Rule.ID != "bearing-wear" || len(routing.Shadows) != 0 {
		t.Fatalf("routing = %+v, want bearing-wear to fire alone", routing)
	}
	recommendation := rules.Recommend(*routing.Fire)
	if recommendation.Parameters["work_order"] != "bearing_wear" || recommendation.Features["current_mean_15m"] != 30.0 ||
		recommendation.PlaybookDigest != rules.Digest() {
		t.Fatalf("recommendation = %+v, want the preset, the context feature and the playbook digest", recommendation)
	}
}

func TestRouteGivesTheModelWhatNoSingleActiveRuleAnswersInsideItsCap(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	cases := map[string]map[string]any{
		"no rule matches":      pumpFeatures(rules, 3.0, 2.0, 30),
		"outside the cap":      pumpFeatures(rules, 7.0, 2.0, 30),
		"context out of range": pumpFeatures(rules, 4.2, 2.0, 40),
		"missing features":     rules.Features(map[string]any{}, nil),
	}
	for name, features := range cases {
		if routing := rules.Route(features, State{}); routing.Fire != nil || routing.RateCapped != nil {
			t.Errorf("%s: routing = %+v, want the model", name, routing)
		}
	}
}

func TestRouteRecordsShadowMatchesAndSkipsOpenCircuits(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	features := pumpFeatures(rules, 3.0, 2.6, 30)
	if routing := rules.Route(features, State{}); len(routing.Shadows) != 1 || routing.Shadows[0].Rule.ID != "misalignment" {
		t.Fatalf("routing = %+v, want the misalignment shadow match", routing)
	}
	open := State{Open: map[RuleKey]bool{{ID: "misalignment", Version: 1}: true, bearingWear: true}}
	if routing := rules.Route(pumpFeatures(rules, 4.2, 2.6, 30), open); routing.Fire != nil || len(routing.Shadows) != 0 {
		t.Fatalf("routing = %+v, want rules with open circuits absent", routing)
	}
}

func TestRouteTripsTheRateCapInsteadOfFiring(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	routing := rules.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{FiresLastHour: map[RuleKey]int{bearingWear: 4}})
	if routing.Fire != nil || routing.RateCapped == nil || routing.RateCapped.Rule.ID != "bearing-wear" {
		t.Fatalf("routing = %+v, want the rate cap reached", routing)
	}
}

func TestRouteGivesTheModelTwoMatchingActiveRules(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	rules.compiled[1].rule.Mode = rules.compiled[0].rule.Mode
	rules.compiled[1].program = rules.compiled[0].program
	if routing := rules.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{}); routing.Fire != nil {
		t.Fatalf("routing = %+v, want no rule when two active rules match", routing)
	}
}

func TestRulesWithoutAPlaybookNeverMatch(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	empty, err := NewRules(rules.spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if routing := empty.Route(pumpFeatures(rules, 4.2, 2.0, 30), State{}); routing.Fire != nil || empty.Digest() != "" {
		t.Fatalf("routing = %+v digest %q, want nothing without a playbook", routing, empty.Digest())
	}
}
