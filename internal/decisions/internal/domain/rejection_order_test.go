package domain

import "testing"

func TestValidateReportsTheEarliestFailingCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(map[string]any)
		reason string
		field  string
	}{
		{"episode before attempt", func(d map[string]any) {
			d["episode_id"] = "epi-other"
			d["attempt_id"] = "att-other"
		}, "snapshot_mismatch", "episode_id"},
		{"attempt before snapshot", func(d map[string]any) {
			d["attempt_id"] = "att-other"
			d["snapshot_digest"] = digestText("1")
		}, "stale_attempt", "attempt_id"},
		{"fence before snapshot", func(d map[string]any) {
			d["fence"] = 99
			d["snapshot_digest"] = digestText("1")
		}, "stale_attempt", "fence"},
		{"snapshot before decision validity", func(d map[string]any) {
			d["snapshot_digest"] = digestText("1")
			d["valid_until"] = "2026-08-12T09:00:00.000000000Z"
		}, "snapshot_mismatch", "snapshot_digest"},
		{"decision binding before intent checks", func(d map[string]any) {
			d["situation_version"] = 3
			firstIntent(d)["type"] = "delete_everything"
		}, "snapshot_mismatch", "situation_version"},
		{"intent binding before intent authority", func(d map[string]any) {
			firstIntent(d)["tenant_id"] = "tenant-other"
			firstIntent(d)["type"] = "delete_everything"
		}, "snapshot_mismatch", "intent.tenant_id"},
		{"intent type before declared risk", func(d map[string]any) {
			firstIntent(d)["type"] = "delete_everything"
			firstIntent(d)["risk_class"] = "R0"
		}, "intent_type_not_allowed", "intent.type"},
		{"intent authority before parameters", func(d map[string]any) {
			firstIntent(d)["risk_class"] = "R0"
			firstIntent(d)["parameters"] = map[string]any{"priority": 42}
		}, "risk_label_mismatch", "intent.risk_class"},
		{"parameter schema before preset", func(d map[string]any) {
			firstIntent(d)["parameters"] = map[string]any{"priority": 42}
		}, "parameter_schema_violation", "intent.parameters"},
		{"preset before intent expiry", func(d map[string]any) {
			firstIntent(d)["parameters"] = map[string]any{"priority": "urgent"}
			firstIntent(d)["expires_at"] = "2026-08-12T09:00:00.000000000Z"
		}, "preset_mismatch", "intent.parameters.priority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			test.mutate(document)
			resealIntents(document)
			_, err := validateDocument(t, document, validInput(t))
			requireRejection(t, err, test.reason, test.field)
		})
	}
}

func TestValidateChecksIntentDigestBeforeIdentityBinding(t *testing.T) {
	t.Parallel()
	document := validDecision()
	firstIntent(document)["tenant_id"] = "tenant-other"
	_, err := validateDocument(t, document, validInput(t))
	requireRejection(t, err, "schema_invalid", "intents[0].intent_digest")
}
