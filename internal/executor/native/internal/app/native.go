package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
)

type Config struct {
	Provider    domain.ModelProvider
	Tools       []domain.Tool
	ToolFactory func(*episodes.Request) []domain.Tool
}

type Executor struct {
	provider    domain.ModelProvider
	tools       map[string]domain.Tool
	maxRepair   uint32
	toolFactory func(*episodes.Request) []domain.Tool
}

var _ episodes.Executor = (*Executor)(nil)

func New(cfg Config) (*Executor, error) {
	if cfg.Provider == nil {
		return nil, errors.New("native provider is required")
	}
	tools := make(map[string]domain.Tool, len(cfg.Tools))
	for _, tool := range cfg.Tools {
		if tool == nil || strings.TrimSpace(tool.Name()) == "" {
			return nil, errors.New("native tool name is required")
		}
		if _, exists := tools[tool.Name()]; exists {
			return nil, fmt.Errorf("duplicate native tool %q", tool.Name())
		}
		tools[tool.Name()] = tool
	}
	return &Executor{provider: cfg.Provider, tools: tools, maxRepair: 1, toolFactory: cfg.ToolFactory}, nil
}

func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	if e == nil || e.provider == nil {
		return nil, errors.New("native executor is not configured")
	}
	if req == nil {
		return nil, errors.New("episode request is required")
	}
	payload, err := domain.DecodeRequest(req.RequestJSON)
	if err != nil {
		return nil, err
	}
	return e.executePayload(ctx, req, payload)
}

func (e *Executor) executePayload(ctx context.Context, req *episodes.Request, payload domain.RequestPayload) (*episodes.Outcome, error) {
	budget, err := domain.RequestBudget(req, payload)
	if err != nil {
		return nil, err
	}
	tools := e.toolsFor(req)
	if budget.WallTime > 0 {
		return e.executeBounded(ctx, req, payload, budget, tools)
	}
	return e.executeLoop(ctx, req, payload, budget, tools)
}

func (e *Executor) toolsFor(req *episodes.Request) map[string]domain.Tool {
	tools := make(map[string]domain.Tool, len(e.tools))
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

func (e *Executor) observe(ctx context.Context, call domain.ToolCall, result domain.ToolResult, budget domain.Budget) (domain.Observation, error) {
	data := result.JSON
	if len(data) == 0 {
		data = []byte(`null`)
	}
	if !json.Valid(data) {
		return domain.Observation{}, fmt.Errorf("tool_result_invalid:%s", call.Name)
	}
	bytesRead := result.Bytes
	if bytesRead == 0 {
		bytesRead = uint64(len(data))
	}
	return e.observationForResult(ctx, call, data, bytesRead, budget)
}

func (e *Executor) observationForResult(ctx context.Context, call domain.ToolCall, data []byte, bytesRead uint64, budget domain.Budget) (domain.Observation, error) {
	if budget.ToolResultBytes > 0 && bytesRead > budget.ToolResultBytes {
		return domain.Observation{}, fmt.Errorf("tool_result_oversized:%s", call.Name)
	}
	return domain.Observation{CallID: call.ID, ToolName: call.Name, ResultJSON: append([]byte(nil), data...), Bytes: bytesRead}, nil
}
