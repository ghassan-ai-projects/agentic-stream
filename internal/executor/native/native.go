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
	// UsageReported distinguishes an explicit zero-usage receipt from a
	// provider response that omitted usage telemetry entirely.
	UsageReported bool
	FinishReason  string
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

func (e *Executor) executeBounded(ctx context.Context, req *episodes.Request, payload requestPayload, budget budgetConfig, tools map[string]Tool) (*episodes.Outcome, error) {
	bounded, cancel := context.WithTimeout(ctx, budget.WallTime)
	defer cancel()
	return e.executeLoop(bounded, req, payload, budget, tools)
}

type budgetConfig struct {
	WallTime             time.Duration
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
	loop := &episodeLoop{
		executor: e, req: req, payload: payload, budget: budget, tools: tools,
		modelReq:  ModelRequest{Episode: req, Prompt: payload.Executor.Prompt, Objective: payload.Executor.Objective, Snapshot: payload.Snapshot, DecisionSchema: payload.Executor.DecisionSchema, AllowedIntentTypes: append([]string(nil), payload.AllowedIntentTypes...), RiskCeiling: payload.RiskCeiling, Tools: toolDefinitions(payload.Tools, tools)},
		seenCalls: make(map[string]struct{}),
	}
	for {
		if outcome := loop.step(ctx); outcome != nil {
			return outcome, nil
		}
	}
}

// episodeLoop is the bounded model/tool loop of one native episode. Each step
// makes one model call and either runs the requested tools, accepts a valid
// Decision, or asks the model to repair an invalid one. Every budget is
// enforced by the loop itself, never by the provider.
type episodeLoop struct {
	executor *Executor
	req      *episodes.Request
	payload  requestPayload
	budget   budgetConfig
	tools    map[string]Tool
	modelReq ModelRequest

	usage                                           Usage
	observations                                    []Observation
	seenCalls                                       map[string]struct{}
	repairs, modelCalls, toolCalls, providerRetries uint32
	toolResultBytes                                 uint64
}

// step runs one model call. It returns the terminal outcome, or nil when the
// loop should continue.
func (l *episodeLoop) step(ctx context.Context) *episodes.Outcome {
	if err := ctx.Err(); err != nil {
		return terminalForContext(l.req, err, l.usage)
	}
	if l.budget.ModelCalls > 0 && l.modelCalls >= l.budget.ModelCalls {
		return failed(l.req, "budget_exhausted:model_calls", l.usage)
	}
	l.modelReq.Observations = append([]Observation(nil), l.observations...)
	l.modelCalls++
	response, err := l.executor.provider.Stream(ctx, l.modelReq)
	if err != nil {
		return l.providerFailure(ctx, err)
	}
	if outcome := l.account(ctx, response); outcome != nil {
		return outcome
	}
	if len(response.ToolCalls) > 0 {
		return l.runTools(ctx, response.ToolCalls)
	}
	return l.decide(response)
}

// providerFailure classifies a failed model call. A retryable failure within
// the retry budget returns nil so the loop calls the model again.
func (l *episodeLoop) providerFailure(ctx context.Context, err error) *episodes.Outcome {
	if errors.Is(err, ErrInterrupt) {
		return failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return terminalForContext(l.req, err, l.usage)
	}
	var retryable *RetryableError
	if !errors.As(err, &retryable) {
		return failed(l.req, "provider_failed", l.usage)
	}
	if l.providerRetries < l.budget.ProviderRetries {
		l.providerRetries++
		if waitErr := waitProviderRetry(ctx); waitErr != nil {
			return terminalForContext(l.req, waitErr, l.usage)
		}
		return nil
	}
	if l.budget.ProviderRetries > 0 {
		return failed(l.req, "budget_exhausted:provider_retries", l.usage)
	}
	return failed(l.req, "provider_failed", l.usage)
}

// account settles the response's usage and enforces the usage budgets and
// wall time.
func (l *episodeLoop) account(ctx context.Context, response ModelResponse) *episodes.Outcome {
	// Count usage before checking cancellation: a provider may return a late
	// response after its context was canceled, but that completed call still
	// consumed provider resources and must be settled.
	l.usage = addUsage(l.usage, response.Usage)
	if hasUsageBudget(l.budget) && !responseUsageReported(response) {
		return failed(l.req, "budget_telemetry_missing", l.usage)
	}
	// A provider is expected to honor cancellation, but a hard wall-time
	// budget must also win when an adapter returns a late response.
	if err := ctx.Err(); err != nil {
		return terminalForContext(l.req, err, l.usage)
	}
	if err := checkUsage(l.usage, l.budget); err != nil {
		return failed(l.req, err.Error(), l.usage)
	}
	return nil
}

// runTools executes the requested tool calls as observations for the next
// model call. Only allow-listed tools with valid, never-repeated arguments
// run, within the tool-call and result-byte budgets.
func (l *episodeLoop) runTools(ctx context.Context, calls []ToolCall) *episodes.Outcome {
	if l.budget.ToolCalls > 0 && uint32(len(calls)) > l.budget.ToolCalls-l.toolCalls { //nolint:gosec // bounded by provider response and checked below.
		return failed(l.req, "budget_exhausted:tool_calls", l.usage)
	}
	for _, call := range calls {
		l.toolCalls++
		if err := ctx.Err(); err != nil {
			return terminalForContext(l.req, err, l.usage)
		}
		if outcome := l.runTool(ctx, call); outcome != nil {
			return outcome
		}
	}
	return nil
}

func (l *episodeLoop) runTool(ctx context.Context, call ToolCall) *episodes.Outcome {
	tool, ok := l.tools[call.Name]
	if !ok {
		return failed(l.req, "tool_not_allowed:"+call.Name, l.usage)
	}
	if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
		return failed(l.req, "tool_arguments_invalid:"+call.Name, l.usage)
	}
	digest := sha256.Sum256(append([]byte(call.Name+"|"), call.Arguments...))
	key := hex.EncodeToString(digest[:])
	if _, exists := l.seenCalls[key]; exists {
		return failed(l.req, "repeated_tool_call:"+call.Name, l.usage)
	}
	l.seenCalls[key] = struct{}{}
	result, err := tool.Call(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, ErrInterrupt) {
			return failed(l.req, "interrupt_in_non_interactive_episode", l.usage)
		}
		l.observations = append(l.observations, Observation{CallID: call.ID, ToolName: call.Name, ErrorCode: "tool_failed"})
		return nil
	}
	observation, err := l.executor.observe(ctx, call, result, l.budget)
	if err != nil {
		return failed(l.req, err.Error(), l.usage)
	}
	l.toolResultBytes += observation.Bytes
	if l.budget.TotalToolResultBytes > 0 && l.toolResultBytes > l.budget.TotalToolResultBytes {
		return failed(l.req, "budget_exhausted:total_tool_result_bytes", l.usage)
	}
	l.observations = append(l.observations, observation)
	return nil
}

// decide accepts a valid Decision as the produced outcome. An invalid one is
// sent back for repair until the repair budget is spent.
func (l *episodeLoop) decide(response ModelResponse) *episodes.Outcome {
	if len(response.DecisionJSON) == 0 {
		return failed(l.req, "provider_returned_no_decision", l.usage)
	}
	decision, validationErr := validateDecision(l.req, response.DecisionJSON, l.payload.AllowedIntentTypes)
	if validationErr == nil {
		return l.produced(decision)
	}
	if l.repairs >= l.executor.maxRepair {
		return failed(l.req, "decision_rejected:"+validationErr.Error(), l.usage)
	}
	l.repairs++
	l.modelReq.Repair = true
	l.modelReq.RepairReason = validationErr.Error()
	return nil
}

func (l *episodeLoop) produced(decision map[string]any) *episodes.Outcome {
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return failed(l.req, "decision_digest_failed", l.usage)
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return failed(l.req, "decision_canonicalization_failed", l.usage)
	}
	return &episodes.Outcome{Status: string(episodes.AttemptProduced), AttemptID: l.req.AttemptID, Fence: l.req.Fence, DecisionJSON: decisionJSON, DecisionSHA256: digest, CostMicrounits: l.usage.CostMicrounits}
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

func addUsage(a, b Usage) Usage {
	return Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CostMicrounits: a.CostMicrounits + b.CostMicrounits}
}

func hasUsageBudget(budget budgetConfig) bool {
	return budget.InputTokens > 0 || budget.OutputTokens > 0 || budget.CostMicrounits > 0
}

func responseUsageReported(response ModelResponse) bool {
	return response.UsageReported || response.Usage != (Usage{})
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

const providerRetryBackoff = 10 * time.Millisecond

func waitProviderRetry(ctx context.Context) error {
	timer := time.NewTimer(providerRetryBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("provider retry backoff canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func terminalForContext(req *episodes.Request, err error, usage Usage) *episodes.Outcome {
	if errors.Is(err, context.DeadlineExceeded) {
		return failed(req, "timed_out", usage)
	}
	return &episodes.Outcome{Status: string(episodes.AttemptCancelled), AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{"canceled"}, CostMicrounits: usage.CostMicrounits}
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
	return ModelResponse{DecisionJSON: raw, Usage: Usage{InputTokens: uint64(len(req.Prompt) + len(req.Objective)), OutputTokens: uint64(len(raw))}, UsageReported: true, FinishReason: "stop"}, nil
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
