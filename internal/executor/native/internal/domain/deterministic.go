package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	return ArtifactRef{ID: id, MediaType: "application/json", SizeBytes: uint64(len(data)), SHA256: canonicaljson.EncodeDigest(digest[:])}, nil
}

// Get returns a copy of an in-memory artifact.
func (s *MemoryArtifactStore) Get(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.items[id]
	return append([]byte(nil), data...), ok
}
