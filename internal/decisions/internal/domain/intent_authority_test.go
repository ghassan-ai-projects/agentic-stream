package domain

import "testing"

func TestValidateRefusesIntentTypesTheEpisodeDoesNotAllow(t *testing.T) {
	t.Parallel()
	document := validDecision()
	firstIntent(document)["type"] = "delete_everything"
	resealIntents(document)
	_, err := validateDocument(t, document, validInput(t))
	requireRejection(t, err, "intent_type_not_allowed", "intent.type")
}

func TestValidateRefusesAllowedTypesTheCatalogDoesNotDeclare(t *testing.T) {
	t.Parallel()
	document := validDecision()
	firstIntent(document)["type"] = "ghost_action"
	resealIntents(document)
	_, err := validateDocument(t, document, inputAllowing(t, "ghost_action"))
	requireRejection(t, err, "intent_type_not_in_catalog", "intent.type")
}

func TestValidateHoldsProposedRiskToTheCatalogAndTheCeiling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		intent string
		risk   string
		reason string
	}{
		{"label below the declared risk", "create_ticket", "R0", "risk_label_mismatch"},
		{"label above the declared risk", "create_ticket", "R2", "risk_label_mismatch"},
		{"declared risk above the ceiling", "schedule_crew", "R2", "risk_ceiling_exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			firstIntent(document)["type"] = test.intent
			firstIntent(document)["risk_class"] = test.risk
			resealIntents(document)
			_, err := validateDocument(t, document, inputAllowing(t, test.intent))
			requireRejection(t, err, test.reason, "intent.risk_class")
		})
	}
}

func TestValidateAllowsCompensationOnlyInReconsiderEpisodes(t *testing.T) {
	t.Parallel()
	compensation := func(withCompensates bool) map[string]any {
		document := validDecision()
		intent := firstIntent(document)
		intent["type"] = "downgrade_maintenance_ticket"
		if withCompensates {
			intent["compensates"] = "cmd-original"
		}
		resealIntents(document)
		return document
	}
	tests := []struct {
		name            string
		withCompensates bool
		reconsider      bool
		wantField       string
	}{
		{"compensation in a reconsider episode", true, true, ""},
		{"compensation outside a reconsider episode", true, false, "intent.compensates"},
		{"plain proposal of a type the episode does not allow", false, true, "intent.type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := validInput(t)
			input.Reconsider = test.reconsider
			result, err := validateDocument(t, compensation(test.withCompensates), input)
			if test.wantField != "" {
				requireRejection(t, err, "intent_type_not_allowed", test.wantField)
				return
			}
			if err != nil || result.Intents[0].Type != "downgrade_maintenance_ticket" {
				t.Fatalf("result = %+v err = %v, want the compensating intent accepted", result, err)
			}
		})
	}
}
