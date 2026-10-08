package domain

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

var ErrInterrupt = errors.New("episode interrupted")

type RetryableError struct{ Err error }

func (e *RetryableError) Error() string { return "retryable provider error: " + e.Err.Error() }

func (e *RetryableError) Unwrap() error { return e.Err }

type Usage struct {
	InputTokens    uint64
	OutputTokens   uint64
	CostMicrounits uint64
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ModelResponse struct {
	DecisionJSON []byte
	ToolCalls    []ToolCall
	Usage        Usage

	UsageReported bool
	FinishReason  string
}

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

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ModelProvider interface {
	Stream(context.Context, ModelRequest) (ModelResponse, error)
	Name() string
}

type Tool interface {
	Name() string
	Call(context.Context, json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
	JSON  []byte
	Rows  uint64
	Bytes uint64
}

type Observation struct {
	CallID     string
	ToolName   string
	ResultJSON []byte
	ErrorCode  string
	Bytes      uint64
}
