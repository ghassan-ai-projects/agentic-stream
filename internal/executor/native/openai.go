package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleProvider speaks the JSON/SSE subset shared by OpenAI-style
// chat-completions endpoints. It is deliberately a provider adapter only: it
// has no tool execution, credentials beyond its request header, or action
// access.
type OpenAICompatibleProvider struct {
	Endpoint string
	APIKey   string
	Model    string
	Client   *http.Client
}

var defaultOpenAIHTTPClient = &http.Client{
	Timeout: 2 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// Name returns the provider adapter identity.
func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible" }

// Stream sends one bounded structured-output turn and normalizes either a
// regular JSON response or Server-Sent Events into ModelResponse.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if p == nil || strings.TrimSpace(p.Endpoint) == "" || strings.TrimSpace(p.Model) == "" {
		return ModelResponse{}, errors.New("openai-compatible endpoint and model are required")
	}
	httpRequest, err := p.buildHTTPRequest(ctx, req)
	if err != nil {
		return ModelResponse{}, err
	}
	client, err := p.boundedHTTPClient()
	if err != nil {
		return ModelResponse{}, err
	}
	return callProvider(client, httpRequest)
}

func (p *OpenAICompatibleProvider) buildHTTPRequest(ctx context.Context, req ModelRequest) (*http.Request, error) {
	userContent, err := buildUserContent(req)
	if err != nil {
		return nil, fmt.Errorf("build provider user content: %w", err)
	}
	body, err := json.Marshal(openAIRequest{Model: p.Model, Stream: true, StreamOptions: map[string]any{"include_usage": true}, Messages: []openAIMessage{
		{Role: "system", Content: req.Prompt},
		{Role: "user", Content: userContent},
	}, Tools: providerTools(req.Tools), ResponseFormat: map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "decision", "strict": true, "schema": json.RawMessage(req.DecisionSchema)}}})
	if err != nil {
		return nil, fmt.Errorf("marshal provider request: %w", err)
	}
	return p.providerHTTPRequest(ctx, body)
}

func (p *OpenAICompatibleProvider) boundedHTTPClient() (*http.Client, error) {
	client := p.Client
	if client == nil {
		client = defaultOpenAIHTTPClient
	}
	if client.Timeout <= 0 {
		return nil, errors.New("openai-compatible HTTP client timeout is required")
	}
	return client, nil
}

type openAIRequest struct {
	Model          string          `json:"model"`
	Stream         bool            `json:"stream"`
	StreamOptions  map[string]any  `json:"stream_options,omitempty"`
	Messages       []openAIMessage `json:"messages"`
	Tools          []openAITool    `json:"tools,omitempty"`
	ResponseFormat map[string]any  `json:"response_format"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildUserContent(req ModelRequest) (string, error) {
	document := map[string]any{
		"objective":            req.Objective,
		"snapshot":             req.Snapshot,
		"allowed_intent_types": req.AllowedIntentTypes,
		"risk_ceiling":         req.RiskCeiling,
		"observations":         req.Observations,
	}
	if req.Repair {
		document["repair_reason"] = req.RepairReason
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal user content: %w", err)
	}
	return string(raw), nil
}

func providerTools(definitions []ToolDefinition) []openAITool {
	result := make([]openAITool, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, openAITool{Type: "function", Function: struct {
			Name        string          `json:"name"`
			Description string          `json:"description,omitempty"`
			Parameters  json.RawMessage `json:"parameters"`
		}{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}})
	}
	return result
}

func callProvider(client *http.Client, httpRequest *http.Request) (ModelResponse, error) {
	response, err := client.Do(httpRequest)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("call model provider: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if err := checkProviderStatus(response); err != nil {
		return ModelResponse{}, err
	}
	return decodeProviderResponse(response)
}

func (p *OpenAICompatibleProvider) providerHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create provider request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	return httpRequest, nil
}
