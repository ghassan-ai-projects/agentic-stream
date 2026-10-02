package native

import (
	"context"
	"encoding/json"
	"errors"
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
