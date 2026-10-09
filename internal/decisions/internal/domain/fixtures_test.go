package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

var validationTime = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func validInput(t *testing.T) Input {
	t.Helper()
	return Input{
		EpisodeID:          "epi-1",
		AttemptID:          "att-1",
		Fence:              1,
		TenantID:           "tenant-1",
		SituationID:        "sit-1",
		SituationVersion:   2,
		SnapshotDigest:     digestText("0"),
		AllowedIntentTypes: map[string]struct{}{"create_ticket": {}},
		RiskCeiling:        "R1",
		IntentCatalog:      compileCatalog(t, testCatalog()),
		Now:                validationTime,
	}
}

func inputAllowing(t *testing.T, types ...string) Input {
	t.Helper()
	input := validInput(t)
	input.AllowedIntentTypes = make(map[string]struct{}, len(types))
	for _, intentType := range types {
		input.AllowedIntentTypes[intentType] = struct{}{}
	}
	return input
}

func compileCatalog(t *testing.T, entries []map[string]any) *IntentCatalog {
	t.Helper()
	catalog, err := CompileIntentCatalog(entries)
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	return catalog
}

func prioritySchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"priority": map[string]any{"type": "string"}},
	}
}

func testCatalog() []map[string]any {
	return []map[string]any{
		{
			"type": "create_ticket", "risk_class": "R1",
			"parameter_schema": prioritySchema(),
			"presets":          map[string]any{"default": map[string]any{"priority": "routine"}},
			"rate_limit":       map[string]any{"per_hour": 6.0},
			"policy":           map[string]any{"requires_approval": true},
		},
		{
			"type": "schedule_crew", "risk_class": "R2",
			"parameter_schema":      prioritySchema(),
			"model_writable_fields": []any{"priority"},
		},
		{
			"type": "downgrade_maintenance_ticket", "risk_class": "R1",
			"parameter_schema":      prioritySchema(),
			"model_writable_fields": []any{"priority"},
		},
		{
			"type": "install_watch_condition", "risk_class": "R1",
			"parameter_schema":      prioritySchema(),
			"model_writable_fields": []any{"priority"},
		},
	}
}

func validDecision() map[string]any {
	return map[string]any{
		"decision_id":       "dec-1",
		"episode_id":        "epi-1",
		"attempt_id":        "att-1",
		"fence":             1,
		"snapshot_digest":   digestText("0"),
		"situation_id":      "sit-1",
		"situation_version": 2,
		"confidence":        0.8,
		"summary":           "maintain the motor",
		"intents":           []any{validIntent("int-1")},
	}
}

func validIntent(id string) map[string]any {
	intent := map[string]any{
		"intent_id":         id,
		"decision_id":       "dec-1",
		"tenant_id":         "tenant-1",
		"situation_id":      "sit-1",
		"situation_version": 2,
		"type":              "create_ticket",
		"risk_class":        "R1",
		"parameters":        map[string]any{"priority": "routine"},
		"expires_at":        "2026-08-12T11:00:00.000000000Z",
	}
	sealIntent(intent)
	return intent
}

func sealIntent(intent map[string]any) {
	delete(intent, "intent_digest")
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		panic(err)
	}
	intent["intent_digest"] = digest
}

func intentsOf(document map[string]any) []any {
	return document["intents"].([]any)
}

func firstIntent(document map[string]any) map[string]any {
	return intentsOf(document)[0].(map[string]any)
}

func resealIntents(document map[string]any) {
	for _, intent := range intentsOf(document) {
		sealIntent(intent.(map[string]any))
	}
}

func encodeDecision(t *testing.T, document map[string]any) ([]byte, string) {
	t.Helper()
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

func validateDocument(t *testing.T, document map[string]any, input Input) (*Result, error) {
	t.Helper()
	raw, digest := encodeDecision(t, document)
	return Validate(raw, digest, input)
}

func requireRejection(t *testing.T, err error, reason, field string) {
	t.Helper()
	var rejection *ValidationError
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want rejection %s (%s)", err, reason, field)
	}
	if rejection.Reason != reason || rejection.Details["field"] != field {
		t.Fatalf("rejection = %s (%v), want %s (%s)", rejection.Reason, rejection.Details["field"], reason, field)
	}
}

func digestText(hexDigit string) string {
	return "sha256:" + strings.Repeat(hexDigit, 64)
}
