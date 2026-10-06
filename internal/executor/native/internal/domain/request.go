package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

type RequestPayload struct {
	Snapshot map[string]any `json:"snapshot"`
	Executor struct {
		Prompt         string          `json:"prompt"`
		Objective      string          `json:"objective"`
		DecisionSchema json.RawMessage `json:"decision_schema"`
	} `json:"executor"`
	Tools              []map[string]any `json:"tools"`
	AllowedIntentTypes []string         `json:"allowed_intent_types"`
	RiskCeiling        string           `json:"risk_ceiling"`
	Budget             struct {
		ModelCalls           uint32 `json:"model_calls"`
		InputTokens          uint64 `json:"input_tokens"`
		OutputTokens         uint64 `json:"output_tokens"`
		ToolCalls            uint32 `json:"tool_calls"`
		ToolResultBytes      uint64 `json:"tool_result_bytes"`
		TotalToolResultBytes uint64 `json:"total_tool_result_bytes"`
		ProviderRetries      uint32 `json:"provider_retries"`
		CostMicrounits       uint64 `json:"cost_microunits"`
	} `json:"budget"`
}

func RequestBudget(req *episodes.Request, payload RequestPayload) (Budget, error) {
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return Budget{}, fmt.Errorf("validate episode budget: %w", err)
	}
	budget := Budget{
		WallTime: wallTime, ModelCalls: payload.Budget.ModelCalls,
		InputTokens: payload.Budget.InputTokens, OutputTokens: payload.Budget.OutputTokens,
		ToolCalls: payload.Budget.ToolCalls, ToolResultBytes: payload.Budget.ToolResultBytes,
		TotalToolResultBytes: payload.Budget.TotalToolResultBytes, ProviderRetries: payload.Budget.ProviderRetries,
		CostMicrounits: payload.Budget.CostMicrounits,
	}
	if budget.WallTime <= 0 && budget.ModelCalls == 0 {
		return Budget{}, errors.New("finite episode budget requires wall_time or model_calls")
	}
	return budget, nil
}

func DecodeRequest(raw []byte) (RequestPayload, error) {
	var payload RequestPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("decode native episode request: %w", err)
	}
	if payload.Snapshot == nil || payload.Executor.DecisionSchema == nil {
		return payload, errors.New("native episode request requires snapshot and decision schema")
	}
	return payload, nil
}

func ToolDefinitions(raw []map[string]any, tools map[string]Tool) []ToolDefinition {
	result := make([]ToolDefinition, 0, len(tools))
	seen := make(map[string]struct{})
	for _, item := range raw {
		name := configuredToolName(item)
		if !acceptToolDefinition(name, tools, seen) {
			continue
		}
		result = append(result, configuredToolDefinition(name, item))
	}
	slices.SortFunc(result, func(a, b ToolDefinition) int { return strings.Compare(a.Name, b.Name) })
	return result
}

func configuredToolName(item map[string]any) string {
	name, _ := item["name"].(string)
	if name == "" {
		name, _ = item["type"].(string)
	}
	return name
}

func acceptToolDefinition(name string, tools map[string]Tool, seen map[string]struct{}) bool {
	if name == "" {
		return false
	}
	if _, ok := tools[name]; !ok {
		return false
	}
	if _, ok := seen[name]; ok {
		return false
	}
	seen[name] = struct{}{}
	return true
}

func configuredToolDefinition(name string, item map[string]any) ToolDefinition {
	description, _ := item["description"].(string)
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	if schema, ok := item["schema"].(map[string]any); ok {
		if encoded, err := json.Marshal(schema); err == nil {
			parameters = encoded
		}
	}
	return ToolDefinition{Name: name, Description: description, Parameters: parameters}
}

func ValidateDecision(req *episodes.Request, raw []byte, allowed []string) (map[string]any, error) {
	var decision map[string]any
	if err := json.Unmarshal(raw, &decision); err != nil {
		return nil, errors.New("decision_json_invalid")
	}
	if decision["episode_id"] != req.EpisodeID || decision["attempt_id"] != req.AttemptID || Number(decision["fence"]) != float64(req.Fence) || decision["situation_id"] != req.SituationID || Number(decision["situation_version"]) != float64(req.SituationVersion) || decision["snapshot_digest"] != req.SnapshotSHA256 {
		return nil, errors.New("decision_identity_mismatch")
	}
	if err := validateAllowedIntents(decision, allowed); err != nil {
		return nil, err
	}
	return decision, nil
}

func validateAllowedIntents(decision map[string]any, allowed []string) error {
	if intents, ok := decision["intents"].([]any); ok {
		for _, rawIntent := range intents {
			intent, ok := rawIntent.(map[string]any)
			if !ok {
				return errors.New("intent_invalid")
			}
			typeName, _ := intent["type"].(string)
			if !slices.Contains(allowed, typeName) {
				return fmt.Errorf("intent_not_allowed:%s", typeName)
			}
		}
	}
	return nil
}
