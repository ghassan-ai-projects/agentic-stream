package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

func TestCheckUsageNamesTheExhaustedBudget(t *testing.T) {
	t.Parallel()
	budget := Budget{InputTokens: 10, OutputTokens: 10, CostMicrounits: 10}
	for want, usage := range map[string]Usage{
		"budget_exhausted:input_tokens":    {InputTokens: 11},
		"budget_exhausted:output_tokens":   {OutputTokens: 11},
		"budget_exhausted:cost_microunits": {CostMicrounits: 11},
	} {
		if err := CheckUsage(usage, budget); err == nil || err.Error() != want {
			t.Errorf("usage %+v = %v, want %s", usage, err, want)
		}
	}
	if err := CheckUsage(Usage{InputTokens: 10}, budget); err != nil {
		t.Fatalf("usage at the ceiling must pass: %v", err)
	}
	if err := CheckUsage(Usage{InputTokens: 1 << 40}, Budget{}); err != nil {
		t.Fatalf("no budget must pass: %v", err)
	}
}

func TestUsageAccountingAndTelemetryRules(t *testing.T) {
	t.Parallel()
	if got := AddUsage(Usage{InputTokens: 1, CostMicrounits: 2}, Usage{InputTokens: 3, OutputTokens: 4}); got != (Usage{InputTokens: 4, OutputTokens: 4, CostMicrounits: 2}) {
		t.Fatalf("sum = %+v", got)
	}
	if HasUsageBudget(Budget{}) || !HasUsageBudget(Budget{OutputTokens: 1}) {
		t.Fatal("usage budget detection is wrong")
	}
	if ResponseUsageReported(ModelResponse{}) || !ResponseUsageReported(ModelResponse{UsageReported: true}) || !ResponseUsageReported(ModelResponse{Usage: Usage{InputTokens: 1}}) {
		t.Fatal("usage telemetry detection is wrong")
	}
}

func TestFailedOutcomeBindsAttemptIdentityAndCost(t *testing.T) {
	t.Parallel()
	req := &episodes.Request{AttemptID: "att", Fence: 3}
	if failed := Failed(req, "why", Usage{CostMicrounits: 7}); failed.Status != "failed" || failed.AttemptID != "att" || failed.Fence != 3 || failed.Reasons[0] != "why" || failed.CostMicrounits != 7 {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestDecodeRequestRequiresSnapshotAndDecisionSchema(t *testing.T) {
	t.Parallel()
	if _, err := DecodeRequest([]byte("broken")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := DecodeRequest([]byte(`{"snapshot":{}}`)); err == nil || !strings.Contains(err.Error(), "requires snapshot and decision schema") {
		t.Fatalf("missing schema = %v", err)
	}
	payload, err := DecodeRequest([]byte(`{"snapshot":{"phase":"x"},"executor":{"decision_schema":{}},"budget":{"model_calls":2}}`))
	if err != nil || payload.Budget.ModelCalls != 2 {
		t.Fatalf("payload %+v err %v", payload, err)
	}
}

func TestDeterministicProviderAndMemoryStore(t *testing.T) {
	t.Parallel()
	provider := &DeterministicProvider{Responses: []ModelResponse{{FinishReason: "scripted"}}}
	req := ModelRequest{Episode: &episodes.Request{EpisodeID: "epi", AttemptID: "att", SnapshotSHA256: "sha256:x"}, Prompt: "p", Objective: "o"}
	first, err := provider.Stream(t.Context(), req)
	if err != nil || first.FinishReason != "scripted" {
		t.Fatalf("scripted = %+v err %v", first, err)
	}
	second, err := provider.Stream(t.Context(), req)
	if err != nil || len(second.DecisionJSON) == 0 || !second.UsageReported || provider.Name() != "deterministic" {
		t.Fatalf("default = %+v err %v", second, err)
	}
}

func TestRetryableErrorUnwrapsItsCause(t *testing.T) {
	t.Parallel()
	root := errors.New("root")
	err := &RetryableError{Err: root}
	if !errors.Is(err, root) || !strings.Contains(err.Error(), "retryable provider error") {
		t.Fatalf("error = %v", err)
	}
}

type namedTool string

func (n namedTool) Name() string { return string(n) }

func (namedTool) Call(context.Context, json.RawMessage) (ToolResult, error) { return ToolResult{}, nil }

func TestValidateDecisionBindsIdentityAndAllowedIntents(t *testing.T) {
	t.Parallel()
	req := &episodes.Request{EpisodeID: "epi", AttemptID: "att", Fence: 2, SituationID: "sit", SituationVersion: 1, SnapshotSHA256: "sha256:s"}
	valid := `{"episode_id":"epi","attempt_id":"att","fence":2,"situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:s","intents":[{"type":"ticket"}]}`
	if _, err := ValidateDecision(req, []byte(valid), []string{"ticket"}); err != nil {
		t.Fatalf("valid decision: %v", err)
	}
	for want, raw := range map[string]string{
		"decision_json_invalid":      "broken",
		"decision_identity_mismatch": `{"episode_id":"other"}`,
		"intent_not_allowed:ticket":  valid,
		"intent_invalid":             `{"episode_id":"epi","attempt_id":"att","fence":2,"situation_id":"sit","situation_version":1,"snapshot_digest":"sha256:s","intents":["x"]}`,
	} {
		allowed := []string{"other"}
		if _, err := ValidateDecision(req, []byte(raw), allowed); err == nil || err.Error() != want {
			t.Errorf("%s: got %v", want, err)
		}
	}
}

func TestToolDefinitionsKeepOnlyConfiguredAllowListedToolsSorted(t *testing.T) {
	t.Parallel()
	tools := map[string]Tool{"b": namedTool("b"), "a": namedTool("a")}
	raw := []map[string]any{{"name": "b"}, {"name": "a"}, {"name": "missing"}, {"name": "a"}, {}}
	definitions := ToolDefinitions(raw, tools)
	if len(definitions) != 2 || definitions[0].Name != "a" || definitions[1].Name != "b" {
		t.Fatalf("definitions = %+v", definitions)
	}
}

func TestEvidenceQueryWindowDefaultsToTheScopeWindowBeforeNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	query, err := EvidenceScope{EntityID: "motor-1", MaxRows: 10, MaxBytes: 1024, Window: 6 * time.Hour}.Query(json.RawMessage(`{}`), now)
	if err != nil || !query.From.Equal(now.Add(-6*time.Hour)) || !query.Until.Equal(now) {
		t.Fatalf("query = %+v, err = %v", query, err)
	}
}
