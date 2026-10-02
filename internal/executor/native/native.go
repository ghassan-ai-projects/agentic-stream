// Package native implements the in-process Go episode executor.
//
// The executor owns the bounded model/tool loop. Providers only propose model
// output; tools are read-only capabilities and every Decision still returns to
// the episodes and policy packages for authoritative validation.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

// Config controls the provider and read-only capabilities used by the native
// loop. Episode resource ceilings come from each trusted Request. Structured
// output repair is always limited to one attempt.
type Config struct {
	Provider      ModelProvider
	Tools         []Tool
	ArtifactStore ArtifactStore
	ToolFactory   func(*episodes.Request) []Tool
}

// Executor is a bounded native Go episode executor.
type Executor struct {
	provider    ModelProvider
	tools       map[string]Tool
	artifacts   ArtifactStore
	maxRepair   uint32
	toolFactory func(*episodes.Request) []Tool
}

// New creates a native executor and rejects duplicate or empty tool names.
func New(cfg Config) (*Executor, error) {
	if cfg.Provider == nil {
		return nil, errors.New("native provider is required")
	}
	tools := make(map[string]Tool, len(cfg.Tools))
	for _, tool := range cfg.Tools {
		if tool == nil || strings.TrimSpace(tool.Name()) == "" {
			return nil, errors.New("native tool name is required")
		}
		if _, exists := tools[tool.Name()]; exists {
			return nil, fmt.Errorf("duplicate native tool %q", tool.Name())
		}
		tools[tool.Name()] = tool
	}
	return &Executor{provider: cfg.Provider, tools: tools, artifacts: cfg.ArtifactStore, maxRepair: 1, toolFactory: cfg.ToolFactory}, nil
}

// Execute runs the provider/read-tool loop and returns a typed attempt
// terminal. It never mutates episode, situation, or action state.
func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	if e == nil || e.provider == nil {
		return nil, errors.New("native executor is not configured")
	}
	if req == nil {
		return nil, errors.New("episode request is required")
	}
	payload, err := decodeRequest(req.RequestJSON)
	if err != nil {
		return nil, err
	}
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	budget := budgetConfig{
		WallTime: wallTime, ModelCalls: payload.Budget.ModelCalls,
		InputTokens: payload.Budget.InputTokens, OutputTokens: payload.Budget.OutputTokens,
		ToolCalls: payload.Budget.ToolCalls, ToolResultBytes: payload.Budget.ToolResultBytes,
		TotalToolResultBytes: payload.Budget.TotalToolResultBytes, ProviderRetries: payload.Budget.ProviderRetries,
		CostMicrounits: payload.Budget.CostMicrounits,
	}
	if budget.WallTime <= 0 && budget.ModelCalls == 0 {
		return nil, errors.New("finite episode budget requires wall_time or model_calls")
	}
	tools := e.toolsFor(req)
	if budget.WallTime > 0 {
		return e.executeBounded(ctx, req, payload, budget, tools)
	}
	return e.executeLoop(ctx, req, payload, budget, tools)
}

func (e *Executor) toolsFor(req *episodes.Request) map[string]Tool {
	tools := make(map[string]Tool, len(e.tools))
	for name, tool := range e.tools {
		tools[name] = tool
	}
	if e.toolFactory != nil {
		for _, tool := range e.toolFactory(req) {
			if tool != nil && strings.TrimSpace(tool.Name()) != "" {
				tools[tool.Name()] = tool
			}
		}
	}
	return tools
}

type requestPayload struct {
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

func decodeRequest(raw []byte) (requestPayload, error) {
	var payload requestPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("decode native episode request: %w", err)
	}
	if payload.Snapshot == nil || payload.Executor.DecisionSchema == nil {
		return payload, errors.New("native episode request requires snapshot and decision schema")
	}
	return payload, nil
}

func toolDefinitions(raw []map[string]any, tools map[string]Tool) []ToolDefinition {
	result := make([]ToolDefinition, 0, len(tools))
	seen := make(map[string]struct{})
	for _, item := range raw {
		name, _ := item["name"].(string)
		if name == "" {
			name, _ = item["type"].(string)
		}
		if name == "" {
			continue
		}
		if _, ok := tools[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		description, _ := item["description"].(string)
		parameters := json.RawMessage(`{"type":"object","additionalProperties":false}`)
		if schema, ok := item["schema"].(map[string]any); ok {
			if encoded, err := json.Marshal(schema); err == nil {
				parameters = encoded
			}
		}
		result = append(result, ToolDefinition{Name: name, Description: description, Parameters: parameters})
	}
	slices.SortFunc(result, func(a, b ToolDefinition) int { return strings.Compare(a.Name, b.Name) })
	return result
}

func (e *Executor) observe(ctx context.Context, call ToolCall, result ToolResult, budget budgetConfig) (Observation, error) {
	data := result.JSON
	if len(data) == 0 {
		data = []byte(`null`)
	}
	if !json.Valid(data) {
		return Observation{}, fmt.Errorf("tool_result_invalid:%s", call.Name)
	}
	bytesRead := result.Bytes
	if bytesRead == 0 {
		bytesRead = uint64(len(data))
	}
	if budget.ToolResultBytes > 0 && bytesRead > budget.ToolResultBytes {
		if e.artifacts == nil {
			return Observation{}, fmt.Errorf("tool_result_oversized:%s", call.Name)
		}
		ref, err := e.artifacts.Put(ctx, data)
		if err != nil {
			return Observation{}, fmt.Errorf("store_tool_artifact:%w", err)
		}
		return Observation{CallID: call.ID, ToolName: call.Name, Artifact: &ref, Bytes: bytesRead}, nil
	}
	return Observation{CallID: call.ID, ToolName: call.Name, ResultJSON: append([]byte(nil), data...), Bytes: bytesRead}, nil
}

func validateDecision(req *episodes.Request, raw []byte, allowed []string) (map[string]any, error) {
	var decision map[string]any
	if err := json.Unmarshal(raw, &decision); err != nil {
		return nil, errors.New("decision_json_invalid")
	}
	if decision["episode_id"] != req.EpisodeID || decision["attempt_id"] != req.AttemptID || number(decision["fence"]) != float64(req.Fence) || decision["situation_id"] != req.SituationID || number(decision["situation_version"]) != float64(req.SituationVersion) || decision["snapshot_digest"] != req.SnapshotSHA256 {
		return nil, errors.New("decision_identity_mismatch")
	}
	if intents, ok := decision["intents"].([]any); ok {
		for _, rawIntent := range intents {
			intent, ok := rawIntent.(map[string]any)
			if !ok {
				return nil, errors.New("intent_invalid")
			}
			typeName, _ := intent["type"].(string)
			if !slices.Contains(allowed, typeName) {
				return nil, fmt.Errorf("intent_not_allowed:%s", typeName)
			}
		}
	}
	return decision, nil
}

// Ensure the native executor remains a valid episode executor.
var _ episodes.Executor = (*Executor)(nil)
