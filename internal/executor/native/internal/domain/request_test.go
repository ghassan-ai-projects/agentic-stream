package domain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

type namedTool string

func (n namedTool) Name() string { return string(n) }

func (namedTool) Call(context.Context, json.RawMessage) (ToolResult, error) { return ToolResult{}, nil }

func TestDecodeRequestRequiresSnapshotAndDecisionSchema(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"invalid json", "broken", "decode native episode request"},
		{"missing decision schema", `{"snapshot":{}}`, "requires snapshot and decision schema"},
		{"missing snapshot", `{"executor":{"decision_schema":{}}}`, "requires snapshot and decision schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeRequest([]byte(tt.raw)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeRequest() = %v, want error containing %q", err, tt.want)
			}
		})
	}
	payload, err := DecodeRequest([]byte(`{"snapshot":{"phase":"x"},"executor":{"decision_schema":{}},"budget":{"model_calls":2}}`))
	if err != nil || payload.Budget.ModelCalls != 2 || payload.Snapshot["phase"] != "x" {
		t.Fatalf("DecodeRequest() = %+v, %v", payload, err)
	}
}

func TestValidateDecisionBindsIdentityAndAllowedIntents(t *testing.T) {
	t.Parallel()
	req := &episodes.Request{EpisodeID: "epi", AttemptID: "att", Fence: 2, SituationID: "sit", SituationVersion: 1, SnapshotSHA256: "sha256:s"}
	identity := `"episode_id":"epi","attempt_id":"att","fence":2,"situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:s"`
	tests := []struct {
		name    string
		raw     string
		allowed []string
		want    string
	}{
		{"decision with an allowed intent", `{` + identity + `,"intents":[{"type":"ticket"}]}`, []string{"ticket"}, ""},
		{"decision without intents", `{` + identity + `}`, nil, ""},
		{"not json", "broken", []string{"ticket"}, "decision_json_invalid"},
		{"another episode", `{"episode_id":"other"}`, []string{"ticket"}, "decision_identity_mismatch"},
		{"another snapshot", `{"episode_id":"epi","attempt_id":"att","fence":2,"situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:other"}`, nil, "decision_identity_mismatch"},
		{"stale fence", `{"episode_id":"epi","attempt_id":"att","fence":1,"situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:s"}`, nil, "decision_identity_mismatch"},
		{"fence that is not a number", `{"episode_id":"epi","attempt_id":"att","fence":"2","situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:s"}`, nil, "decision_identity_mismatch"},
		{"intent outside the allowed set", `{` + identity + `,"intents":[{"type":"ticket"}]}`, []string{"other"}, "intent_not_allowed:ticket"},
		{"intent that is not an object", `{` + identity + `,"intents":["x"]}`, []string{"ticket"}, "intent_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decision, err := ValidateDecision(req, []byte(tt.raw), tt.allowed)
			if tt.want == "" && (err != nil || decision == nil) || tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Fatalf("ValidateDecision() = %v, %v; want %q", decision, err, tt.want)
			}
		})
	}
}

func TestToolDefinitionsKeepOnlyConfiguredAllowListedToolsSorted(t *testing.T) {
	t.Parallel()
	tools := map[string]Tool{"b": namedTool("b"), "a": namedTool("a"), "typed": namedTool("typed")}
	raw := []map[string]any{
		{"name": "b", "description": "reads b", "schema": map[string]any{"type": "object", "properties": map[string]any{"limit": map[string]any{"type": "integer"}}}},
		{"name": "a"}, {"name": "missing"}, {"name": "a", "description": "duplicate"}, {}, {"type": "typed"},
	}
	definitions := ToolDefinitions(raw, tools)
	if len(definitions) != 3 || definitions[0].Name != "a" || definitions[1].Name != "b" || definitions[2].Name != "typed" {
		t.Fatalf("ToolDefinitions() = %+v, want a, b, typed", definitions)
	}
	if definitions[0].Description != "" || string(definitions[0].Parameters) != `{"type":"object","additionalProperties":false}` {
		t.Errorf("a tool without a schema must take the closed default: %+v", definitions[0])
	}
	if definitions[1].Description != "reads b" || !strings.Contains(string(definitions[1].Parameters), `"limit"`) {
		t.Errorf("b tool lost its description or schema: %+v", definitions[1])
	}
}
