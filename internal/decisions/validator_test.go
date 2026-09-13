package decisions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
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

func TestValidateAllowsExplicitAbstention(t *testing.T) {
	document := validDecision()
	document["decision_type"] = "need_more_evidence"
	document["intents"] = []any{}
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal abstention: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatalf("digest abstention: %v", err)
	}
	result, err := Validate(raw, digest, validInput())
	if err != nil {
		t.Fatalf("Validate abstention: %v", err)
	}
	if len(result.Intents) != 0 {
		t.Fatalf("abstention produced %d intents", len(result.Intents))
	}
}

func TestValidateRejectsImplicitAbstention(t *testing.T) {
	document := validDecision()
	document["intents"] = []any{}
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal implicit abstention: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatalf("digest implicit abstention: %v", err)
	}
	_, err = Validate(raw, digest, validInput())
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Reason != "schema_invalid" {
		t.Fatalf("implicit abstention error = %v, want schema_invalid", err)
	}
}

func TestValidateCompensatingIntentTypes(t *testing.T) {
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatalf("compile predictive-maintenance spec: %v", err)
	}
	specAllowed := make(map[string]struct{}, len(compiled.Actions.Intents))
	for _, configured := range compiled.Actions.Intents {
		specAllowed[configured.Type] = struct{}{}
	}

	tests := []struct {
		name             string
		intentType       string
		useSpecAllowlist bool
		wantErr          string
	}{
		{name: "downgrade allowed by spec", intentType: "downgrade_maintenance_ticket", useSpecAllowlist: true},
		{name: "withdraw allowed by spec", intentType: "withdraw_maintenance_ticket", useSpecAllowlist: true},
		{name: "downgrade not allowed by episode", intentType: "downgrade_maintenance_ticket", wantErr: "intent_type_not_allowed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := validDecision()
			intent := document["intents"].([]any)[0].(map[string]any)
			intent["type"] = test.intentType
			intent["risk_class"] = "R1"
			if !test.useSpecAllowlist {
				// Without compensates this is a plain proposal: the episode
				// allowlist gates it (P4 keeps the per-episode subset).
				delete(intent, "compensates")
			} else {
				intent["compensates"] = "cmd-original"
			}
			refreshIntentDigest(document)

			raw, err := canonicaljson.Marshal(document)
			if err != nil {
				t.Fatalf("marshal decision: %v", err)
			}
			digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
			if err != nil {
				t.Fatalf("digest decision: %v", err)
			}
			input := validInput()
			if test.useSpecAllowlist {
				input.AllowedIntentTypes = specAllowed
			}
			input.Kind = "reconsider"

			result, err := Validate(raw, digest, input)
			if test.wantErr != "" {
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) || validationErr.Reason != test.wantErr {
					t.Fatalf("error = %v, want reason %s", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if len(result.Intents) != 1 || result.Intents[0].Type != test.intentType || result.Intents[0].RiskClass != "R1" {
				t.Fatalf("validated intent = %+v", result.Intents)
			}
		})
	}
}

func TestValidateRejectsSecurityAndBindingFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		input  *Input
		raw    []byte
		want   string
	}{
		{name: "snapshot mismatch", mutate: func(doc map[string]any) { doc["snapshot_digest"] = "sha256:" + ones(64) }, want: "snapshot_mismatch"},
		{name: "intent type", mutate: func(doc map[string]any) { doc["intents"].([]any)[0].(map[string]any)["type"] = "delete_everything" }, want: "intent_type_not_allowed"},
		{name: "risk label attack", mutate: func(doc map[string]any) { doc["intents"].([]any)[0].(map[string]any)["risk_class"] = "R0" }, want: "risk_label_mismatch"},
		{name: "risk ceiling", mutate: func(doc map[string]any) {
			intent := doc["intents"].([]any)[0].(map[string]any)
			intent["type"] = "schedule_crew"
			intent["risk_class"] = "R2"
		}, input: func() *Input { i := inputWithAllowlist("create_ticket", "schedule_crew"); return &i }(), want: "risk_ceiling_exceeded"},
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
			input := validInput()
			if test.input != nil {
				input = *test.input
			}
			_, err = Validate(raw, digest, input)
			var validationErr *ValidationError
			ok := errors.As(err, &validationErr)
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
		var validationErr *ValidationError
		ok := errors.As(err, &validationErr)
		if !ok || validationErr.Reason != "schema_invalid" {
			t.Fatalf("error = %v, want schema_invalid", err)
		}
	})

	t.Run("duplicate raw key", func(t *testing.T) {
		raw := []byte(`{"decision_id":"dec-1","decision_id":"dec-2"}`)
		_, err := Validate(raw, "", validInput())
		var validationErr *ValidationError
		ok := errors.As(err, &validationErr)
		if !ok || validationErr.Reason != "schema_invalid" {
			t.Fatalf("error = %v, want schema_invalid", err)
		}
	})
}

func TestValidateRejectsPresetSchemaAndEvidenceAttacks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "preset field tampered",
			mutate: func(doc map[string]any) {
				doc["intents"].([]any)[0].(map[string]any)["parameters"] = map[string]any{"priority": "urgent"}
			}, want: "preset_mismatch"},
		{name: "schema-violating parameter",
			mutate: func(doc map[string]any) {
				doc["intents"].([]any)[0].(map[string]any)["parameters"] = map[string]any{"priority": 42}
			}, want: "parameter_schema_violation"},
		{name: "ungrounded evidence",
			mutate: func(doc map[string]any) {
				doc["intents"].([]any)[0].(map[string]any)["evidence_ids"] = []any{"fact:forged"}
			}, want: "ungrounded_evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := validDecision()
			document["facts_used"] = []any{map[string]any{"evidence": "fact:dissolved_oxygen"}}
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
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || validationErr.Reason != test.want {
				t.Fatalf("error = %v, want reason %s", err, test.want)
			}
		})
	}
}

func TestValidateRequiresTheCompiledCatalog(t *testing.T) {
	document := validDecision()
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		t.Fatalf("digest decision: %v", err)
	}
	input := validInput()
	input.IntentCatalog = nil
	_, err = Validate(raw, digest, input)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Reason != "catalog_missing" {
		t.Fatalf("error = %v, want catalog_missing", err)
	}
}

func TestCompileIntentCatalogFailsClosed(t *testing.T) {
	if _, err := CompileIntentCatalog(nil); err == nil {
		t.Fatal("an empty catalog must fail compilation")
	}
	if _, err := CompileIntentCatalog([]map[string]any{}); err == nil {
		t.Fatal("an empty catalog must fail compilation")
	}
	duplicate := testCatalog()
	duplicate = append(duplicate, testCatalog()[0])
	if _, err := CompileIntentCatalog(duplicate); err == nil {
		t.Fatal("duplicate types must fail compilation")
	}
	badRisk := []map[string]any{{"type": "x", "risk_class": "R9",
		"parameter_schema": map[string]any{"type": "object"}}}
	if _, err := CompileIntentCatalog(badRisk); err == nil {
		t.Fatal("an invalid risk class must fail compilation")
	}
	noSchema := []map[string]any{{"type": "x", "risk_class": "R1"}}
	if _, err := CompileIntentCatalog(noSchema); err == nil {
		t.Fatal("a missing parameter schema must fail compilation")
	}
}

// TestValidateBindsTargetToEpisodeIdentity guards A-013 F1: an identity-bearing
// target parameter must bind to the dispatched episode's entity even when the
// optional entity_id parameter is absent. Previously target-without-entity_id
// skipped verification, letting a proposal steer an effect at an arbitrary,
// unverified target.
func TestValidateBindsTargetToEpisodeIdentity(t *testing.T) {
	targetSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"target": map[string]any{"type": "string"}},
	}
	catalog, err := CompileIntentCatalog([]map[string]any{
		{"type": "act", "risk_class": "R1", "parameter_schema": targetSchema,
			"model_writable_fields": []any{"target"}},
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}

	build := func(target string) ([]byte, string) {
		intent := map[string]any{
			"intent_id": "int-1", "decision_id": "dec-1", "tenant_id": "tenant-1",
			"situation_id": "sit-1", "situation_version": 2, "type": "act",
			"risk_class": "R1", "parameters": map[string]any{"target": target},
			"expires_at": "2026-08-12T11:00:00.000000000Z",
		}
		document := validDecision()
		document["intents"] = []any{intent}
		refreshIntentDigest(document)
		raw, err := canonicaljson.Marshal(document)
		if err != nil {
			t.Fatalf("marshal decision: %v", err)
		}
		digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
		if err != nil {
			t.Fatalf("digest decision: %v", err)
		}
		return raw, digest
	}

	input := validInput()
	input.EntityID = "motor-7"
	input.AllowedIntentTypes = map[string]struct{}{"act": {}}
	input.IntentCatalog = catalog

	t.Run("mismatched target rejected", func(t *testing.T) {
		raw, digest := build("attacker-controlled-target")
		_, err := Validate(raw, digest, input)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) || validationErr.Reason != "snapshot_mismatch" {
			t.Fatalf("error = %v, want snapshot_mismatch", err)
		}
	})

	t.Run("target missing episode identity rejected", func(t *testing.T) {
		raw, digest := build("motor-7")
		noIdentity := input
		noIdentity.EntityID = ""
		_, err := Validate(raw, digest, noIdentity)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) || validationErr.Reason != "snapshot_mismatch" {
			t.Fatalf("error = %v, want snapshot_mismatch", err)
		}
	})

	t.Run("bound target accepted", func(t *testing.T) {
		raw, digest := build("motor-7")
		if _, err := Validate(raw, digest, input); err != nil {
			t.Fatalf("target bound to episode entity should validate: %v", err)
		}
	})
}

func validInput() Input {
	input := Input{
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
	catalog, err := CompileIntentCatalog(testCatalog())
	if err != nil {
		panic(err)
	}
	input.IntentCatalog = catalog
	return input
}

// testCatalog declares the types the fixtures use: create_ticket (R1, preset-
// authored priority — NOT model-writable, so a tampered value is a preset
// mismatch), schedule_crew (R2, above an R1 ceiling), and the compensation
// types downgrade/withdraw (R1, catalog members per G5).
func testCatalog() []map[string]any {
	prioritySchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"priority": map[string]any{"type": "string"}},
	}
	entries := []map[string]any{
		{
			"type": "create_ticket", "risk_class": "R1",
			"parameter_schema": prioritySchema,
			"presets":          map[string]any{"default": map[string]any{"priority": "routine"}},
		},
		{
			"type": "schedule_crew", "risk_class": "R2",
			"parameter_schema":      prioritySchema,
			"model_writable_fields": []any{"priority"},
		},
		{
			"type": "downgrade_maintenance_ticket", "risk_class": "R1",
			"parameter_schema":      prioritySchema,
			"model_writable_fields": []any{"priority"},
		},
		{
			"type": "withdraw_maintenance_ticket", "risk_class": "R1",
			"parameter_schema":      prioritySchema,
			"model_writable_fields": []any{"priority"},
		},
	}
	return entries
}

func inputWithAllowlist(types ...string) Input {
	input := validInput()
	allowed := make(map[string]struct{}, len(types))
	for _, intentType := range types {
		allowed[intentType] = struct{}{}
	}
	input.AllowedIntentTypes = allowed
	return input
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
