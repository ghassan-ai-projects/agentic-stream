package domain

import (
	"context"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

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
