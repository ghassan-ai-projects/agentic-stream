package decisions

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestValidateBindsDecisionAndIntents(t *testing.T) {
	document := validDecision()
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatalf("digest decision: %v", err)
	}
	result, err := Validate(raw, digest, validInput())
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if result.DecisionID != "dec-1" || len(result.Intents) != 1 {
		t.Fatalf("result = decision %q with %d intents", result.DecisionID, len(result.Intents))
	}
	if result.Intents[0].Digest == "" || result.Intents[0].ID != "int-1" {
		t.Fatalf("intent provenance = %+v", result.Intents[0])
	}
}

func TestValidateRejectsSecurityAndBindingFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		raw    []byte
		want   string
	}{
		{name: "snapshot mismatch", mutate: func(doc map[string]any) { doc["snapshot_digest"] = "sha256:" + ones(64) }, want: "snapshot_mismatch"},
		{name: "intent type", mutate: func(doc map[string]any) { doc["intents"].([]any)[0].(map[string]any)["type"] = "delete_everything" }, want: "intent_type_not_allowed"},
		{name: "risk ceiling", mutate: func(doc map[string]any) { doc["intents"].([]any)[0].(map[string]any)["risk_class"] = "R3" }, want: "risk_ceiling_exceeded"},
		{name: "expired intent", mutate: func(doc map[string]any) {
			doc["intents"].([]any)[0].(map[string]any)["expires_at"] = "2026-08-12T09:00:00.000000000Z"
		}, want: "expired"},
		{name: "duplicate intent id", mutate: func(doc map[string]any) {
			second := doc["intents"].([]any)[0].(map[string]any)
			doc["intents"] = []any{second, second}
		}, want: "schema_invalid"},
		{name: "unknown property", mutate: func(doc map[string]any) { doc["unexpected"] = true }, want: "schema_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := validDecision()
			test.mutate(document)
			refreshIntentDigest(document)
			raw, err := canonicaljson.Marshal(document)
			if err != nil {
				t.Fatalf("marshal mutated decision: %v", err)
			}
			digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
			if err != nil {
				t.Fatalf("digest mutated decision: %v", err)
			}
			_, err = Validate(raw, digest, validInput())
			validationErr, ok := err.(*ValidationError)
			if !ok || validationErr.Reason != test.want {
				t.Fatalf("error = %v, want reason %s", err, test.want)
			}
		})
	}

	t.Run("digest tamper", func(t *testing.T) {
		document := validDecision()
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("marshal decision: %v", err)
		}
		_, err = Validate(raw, "sha256:"+zeros(64), validInput())
		validationErr, ok := err.(*ValidationError)
		if !ok || validationErr.Reason != "schema_invalid" {
			t.Fatalf("error = %v, want schema_invalid", err)
		}
	})

	t.Run("duplicate raw key", func(t *testing.T) {
		raw := []byte(`{"decision_id":"dec-1","decision_id":"dec-2"}`)
		_, err := Validate(raw, "", validInput())
		validationErr, ok := err.(*ValidationError)
		if !ok || validationErr.Reason != "schema_invalid" {
			t.Fatalf("error = %v, want schema_invalid", err)
		}
	})
}

func validInput() Input {
	return Input{
		EpisodeID:          "epi-1",
		AttemptID:          "att-1",
		Fence:              1,
		TenantID:           "tenant-1",
		SituationID:        "sit-1",
		SituationVersion:   2,
		SnapshotDigest:     "sha256:" + zeros(64),
		AllowedIntentTypes: map[string]struct{}{"create_ticket": {}},
		RiskCeiling:        "R1",
		Now:                time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC),
	}
}

func validDecision() map[string]any {
	return map[string]any{
		"decision_id":       "dec-1",
		"episode_id":        "epi-1",
		"attempt_id":        "att-1",
		"fence":             1,
		"snapshot_digest":   "sha256:" + zeros(64),
		"situation_id":      "sit-1",
		"situation_version": 2,
		"confidence":        0.8,
		"summary":           "maintain the motor",
		"intents":           []any{validIntent()},
	}
}

func validIntent() map[string]any {
	intent := map[string]any{
		"intent_id":         "int-1",
		"decision_id":       "dec-1",
		"tenant_id":         "tenant-1",
		"situation_id":      "sit-1",
		"situation_version": 2,
		"type":              "create_ticket",
		"risk_class":        "R1",
		"parameters":        map[string]any{"priority": "routine"},
		"expires_at":        "2026-08-12T11:00:00.000000000Z",
	}
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		panic(err)
	}
	intent["intent_digest"] = digest
	return intent
}

func refreshIntentDigest(document map[string]any) {
	intents, ok := document["intents"].([]any)
	if !ok {
		return
	}
	for _, raw := range intents {
		intent, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		delete(intent, "intent_digest")
		digest, err := contractsv1.IntentDigest(intent)
		if err != nil {
			panic(err)
		}
		intent["intent_digest"] = digest
	}
}

func zeros(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = '0'
	}
	return string(result)
}

func ones(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = '1'
	}
	return string(result)
}
