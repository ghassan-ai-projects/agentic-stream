// Package native implements the in-process Go episode executor.
//
// The executor owns the bounded model/tool loop. Providers only propose model
// output; tools are read-only capabilities and every Decision still returns to
// the episodes and policy packages for authoritative validation.
package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

// ErrInterrupt is a provider or tool interruption. Non-interactive episodes
// fail immediately when this error is returned; they never wait for input.
var ErrInterrupt = errors.New("episode interrupted")

// RetryableError marks a provider failure that may consume the separate
// provider-retry allowance. The model-call and cost budgets still increase.
type RetryableError struct{ Err error }

// Error implements error.
func (e *RetryableError) Error() string { return "retryable provider error: " + e.Err.Error() }

// Unwrap exposes the provider's root failure.
func (e *RetryableError) Unwrap() error { return e.Err }

// Usage is provider-reported usage. The runtime treats these values as
// cumulative consumption and never refunds a failed or repaired call.
type Usage struct {
	InputTokens    uint64
	OutputTokens   uint64
	CostMicrounits uint64
}

// ToolCall is a provider-proposed read operation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ModelResponse is one provider turn. A turn produces either a structured
// Decision or bounded read-tool calls for the next turn.
type ModelResponse struct {
	DecisionJSON []byte
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason string
}

// ModelRequest is the immutable episode projection plus bounded observations.
type ModelRequest struct {
	Episode            *episodes.Request
	Prompt             string
	Objective          string
	Snapshot           map[string]any
	DecisionSchema     json.RawMessage
	AllowedIntentTypes []string
	RiskCeiling        string
	Tools              []ToolDefinition
	Observations       []Observation
	Repair             bool
	RepairReason       string
}

// ToolDefinition is the provider-facing declaration of one read capability.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ModelProvider is the narrow provider port used by the native executor.
type ModelProvider interface {
	Stream(context.Context, ModelRequest) (ModelResponse, error)
	Name() string
}

// Tool is a read-only episode capability. Implementations must not dispatch
// commands, access credentials, or mutate stream state.
type Tool interface {
	Name() string
	Call(context.Context, json.RawMessage) (ToolResult, error)
}

// ToolResult is a bounded structured observation. Large results are moved to
// an ArtifactStore before the next provider turn.
type ToolResult struct {
	JSON  []byte
	Rows  uint64
	Bytes uint64
}

// Observation is the durable-safe projection sent to the provider. Artifact
// observations contain only metadata, never an unbounded result body.
type Observation struct {
	CallID     string
	ToolName   string
	ResultJSON []byte
	Artifact   *ArtifactRef
	ErrorCode  string
	Bytes      uint64
}

// ArtifactRef identifies a bounded externalized tool result.
type ArtifactRef struct {
	ID        string `json:"id"`
	MediaType string `json:"media_type"`
	SizeBytes uint64 `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// ArtifactStore receives oversized tool results. A production implementation
// should back this interface with the runtime artifact repository.
type ArtifactStore interface {
	Put(context.Context, []byte) (ArtifactRef, error)
}

// Config controls hard ceilings for the native loop. Zero means unlimited for
// that dimension. Structured-output repair is always limited to one attempt.
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

// Name returns the stable executor name recorded in episode provenance.
func (e *Executor) Name() string { return "native" }

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
	budget := budgetConfig{
		WallTime: payload.Budget.WallTime, ModelCalls: payload.Budget.ModelCalls,
		InputTokens: payload.Budget.InputTokens, OutputTokens: payload.Budget.OutputTokens,
		ToolCalls: payload.Budget.ToolCalls, ToolResultBytes: payload.Budget.ToolResultBytes,
		TotalToolResultBytes: payload.Budget.TotalToolResultBytes, ProviderRetries: payload.Budget.ProviderRetries,
		CostMicrounits: payload.Budget.CostMicrounits,
	}
	tools := e.toolsFor(req)
	if budget.WallTime != "" {
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
		WallTime             string `json:"wall_time"`
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

func (e *Executor) executeBounded(ctx context.Context, req *episodes.Request, payload requestPayload, budget budgetConfig, tools map[string]Tool) (*episodes.Outcome, error) {
	duration, err := parseWallTime(budget.WallTime)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	return e.executeLoop(bounded, req, payload, budget, tools)
}

func parseWallTime(raw string) (time.Duration, error) {
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid native wall_time %q", raw)
	}
	return duration, nil
}

type budgetConfig = struct {
	WallTime             string
	ModelCalls           uint32
	InputTokens          uint64
	OutputTokens         uint64
	ToolCalls            uint32
	ToolResultBytes      uint64
	TotalToolResultBytes uint64
	ProviderRetries      uint32
	CostMicrounits       uint64
}

func (e *Executor) executeLoop(ctx context.Context, req *episodes.Request, payload requestPayload, budget budgetConfig, tools map[string]Tool) (*episodes.Outcome, error) {
	modelReq := ModelRequest{Episode: req, Prompt: payload.Executor.Prompt, Objective: payload.Executor.Objective, Snapshot: payload.Snapshot, DecisionSchema: payload.Executor.DecisionSchema, AllowedIntentTypes: append([]string(nil), payload.AllowedIntentTypes...), RiskCeiling: payload.RiskCeiling, Tools: toolDefinitions(payload.Tools, tools)}
	var usage Usage
	var observations []Observation
	seenCalls := make(map[string]struct{})
	var repairs uint32
	var modelCalls uint32
	var toolCallsUsed uint32
	var totalToolResultBytes uint64
	var providerRetries uint32
	for {
		if err := ctx.Err(); err != nil {
			return terminalForContext(req, err), nil
		}
		if budget.ModelCalls > 0 && modelCalls >= budget.ModelCalls {
			return failed(req, "budget_exhausted:model_calls", usage), nil
		}
		modelReq.Observations = append([]Observation(nil), observations...)
		modelCalls++
		response, err := e.provider.Stream(ctx, modelReq)
		if err != nil {
			if errors.Is(err, ErrInterrupt) {
				return failed(req, "interrupt_in_non_interactive_episode", usage), nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return terminalForContext(req, err), nil
			}
			var retryable *RetryableError
			if errors.As(err, &retryable) && providerRetries < budget.ProviderRetries {
				providerRetries++
				continue
			}
			if errors.As(err, &retryable) && budget.ProviderRetries > 0 {
				return failed(req, "budget_exhausted:provider_retries", usage), nil
			}
			return failed(req, "provider_failed", usage), nil
		}
		// A provider is expected to honor cancellation, but a hard wall-time
		// budget must also win when an adapter returns a late response.
		if err := ctx.Err(); err != nil {
			return terminalForContext(req, err), nil
		}
		usage = addUsage(usage, response.Usage)
		if err := checkUsage(usage, budget); err != nil {
			return failed(req, err.Error(), usage), nil
		}
		if len(response.ToolCalls) > 0 {
			if budget.ToolCalls > 0 && uint32(len(response.ToolCalls)) > budget.ToolCalls-toolCallsUsed { //nolint:gosec // bounded by provider response and checked below.
				return failed(req, "budget_exhausted:tool_calls", usage), nil
			}
			for _, call := range response.ToolCalls {
				toolCallsUsed++
				if err := ctx.Err(); err != nil {
					return terminalForContext(req, err), nil
				}
				tool, ok := tools[call.Name]
				if !ok {
					return failed(req, "tool_not_allowed:"+call.Name, usage), nil
				}
				if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
					return failed(req, "tool_arguments_invalid:"+call.Name, usage), nil
				}
				digest := sha256.Sum256(append([]byte(call.Name+"|"), call.Arguments...))
				key := hex.EncodeToString(digest[:])
				if _, exists := seenCalls[key]; exists {
					return failed(req, "repeated_tool_call:"+call.Name, usage), nil
				}
				seenCalls[key] = struct{}{}
				result, callErr := tool.Call(ctx, call.Arguments)
				if callErr != nil {
					if errors.Is(callErr, ErrInterrupt) {
						return failed(req, "interrupt_in_non_interactive_episode", usage), nil
					}
					observations = append(observations, Observation{CallID: call.ID, ToolName: call.Name, ErrorCode: "tool_failed"})
					continue
				}
				observation, obsErr := e.observe(ctx, call, result, budget)
				if obsErr != nil {
					return failed(req, obsErr.Error(), usage), nil
				}
				totalToolResultBytes += observation.Bytes
				if budget.TotalToolResultBytes > 0 && totalToolResultBytes > budget.TotalToolResultBytes {
					return failed(req, "budget_exhausted:total_tool_result_bytes", usage), nil
				}
				observations = append(observations, observation)
			}
			continue
		}
		if len(response.DecisionJSON) == 0 {
			return failed(req, "provider_returned_no_decision", usage), nil
		}
		decision, validationErr := validateDecision(req, response.DecisionJSON, payload.AllowedIntentTypes)
		if validationErr == nil {
			digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
			if err != nil {
				return failed(req, "decision_digest_failed", usage), nil
			}
			return &episodes.Outcome{Status: string(episodes.AttemptProduced), AttemptID: req.AttemptID, Fence: req.Fence, DecisionJSON: mustCanonical(decision), DecisionSHA256: digest, CostMicrounits: usage.CostMicrounits}, nil
		}
		if repairs >= e.maxRepair {
			return failed(req, "decision_rejected:"+validationErr.Error(), usage), nil
		}
		repairs++
		modelReq.Repair = true
		modelReq.RepairReason = validationErr.Error()
	}
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

func number(value any) float64 {
	if n, ok := value.(float64); ok {
		return n
	}
	return -1
}

func mustCanonical(document map[string]any) []byte {
	raw, _ := canonicaljson.Marshal(document)
	return raw
}

func addUsage(a, b Usage) Usage {
	return Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CostMicrounits: a.CostMicrounits + b.CostMicrounits}
}
func checkUsage(usage Usage, budget budgetConfig) error {
	if budget.InputTokens > 0 && usage.InputTokens > budget.InputTokens {
		return errors.New("budget_exhausted:input_tokens")
	}
	if budget.OutputTokens > 0 && usage.OutputTokens > budget.OutputTokens {
		return errors.New("budget_exhausted:output_tokens")
	}
	if budget.CostMicrounits > 0 && usage.CostMicrounits > budget.CostMicrounits {
		return errors.New("budget_exhausted:cost_microunits")
	}
	return nil
}

func terminalForContext(req *episodes.Request, err error) *episodes.Outcome {
	if errors.Is(err, context.DeadlineExceeded) {
		return failed(req, "timed_out", Usage{})
	}
	return &episodes.Outcome{Status: string(episodes.AttemptCancelled), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{"canceled"}}
}

func failed(req *episodes.Request, reason string, usage Usage) *episodes.Outcome {
	return &episodes.Outcome{Status: string(episodes.AttemptFailed), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{reason}, CostMicrounits: usage.CostMicrounits}
}

// DeterministicProvider is the built-in provider for replay and tests. It can
// return scripted tool calls, then emits a valid empty-intent Decision.
type DeterministicProvider struct {
	Responses []ModelResponse
	mu        sync.Mutex
	index     int
}

// Name returns the provider identity.
func (p *DeterministicProvider) Name() string { return "deterministic" }

// Stream returns the next scripted response, or a valid deterministic
// Decision when no script is configured.
func (p *DeterministicProvider) Stream(_ context.Context, req ModelRequest) (ModelResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index < len(p.Responses) {
		response := p.Responses[p.index]
		p.index++
		return response, nil
	}
	decision := map[string]any{"decision_id": "dec_" + req.Episode.EpisodeID, "episode_id": req.Episode.EpisodeID, "attempt_id": req.Episode.AttemptID, "fence": req.Episode.Fence, "snapshot_digest": req.Episode.SnapshotSHA256, "situation_id": req.Episode.SituationID, "situation_version": req.Episode.SituationVersion, "summary": "deterministic native decision", "confidence": 1.0, "facts_used": []any{}, "decision_type": "need_more_evidence", "intents": []any{}}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("marshal deterministic decision: %w", err)
	}
	return ModelResponse{DecisionJSON: raw, Usage: Usage{InputTokens: uint64(len(req.Prompt) + len(req.Objective)), OutputTokens: uint64(len(raw))}, FinishReason: "stop"}, nil
}

// MemoryArtifactStore is a bounded test/reference artifact store.
type MemoryArtifactStore struct {
	mu    sync.Mutex
	items map[string][]byte
}

// NewMemoryArtifactStore creates an in-memory artifact store.
func NewMemoryArtifactStore() *MemoryArtifactStore {
	return &MemoryArtifactStore{items: make(map[string][]byte)}
}

// Put stores bytes under their content digest.
func (s *MemoryArtifactStore) Put(_ context.Context, data []byte) (ArtifactRef, error) {
	digest := sha256.Sum256(data)
	id := "artifact-" + hex.EncodeToString(digest[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = append([]byte(nil), data...)
	return ArtifactRef{ID: id, MediaType: "application/json", SizeBytes: uint64(len(data)), SHA256: "sha256:" + hex.EncodeToString(digest[:])}, nil
}

// Get returns a copy of an in-memory artifact.
func (s *MemoryArtifactStore) Get(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.items[id]
	return append([]byte(nil), data...), ok
}

// Ensure the native executor remains a valid episode executor.
var _ episodes.Executor = (*Executor)(nil)
