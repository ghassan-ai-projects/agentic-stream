package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (r *Request) SnapshotFacts() (map[string]any, []string, error) {
	var request struct {
		Snapshot struct {
			Facts    map[string]any `json:"facts"`
			Evidence []string       `json:"evidence"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(r.RequestJSON, &request); err != nil {
		return nil, nil, fmt.Errorf("decode the snapshot of episode %s: %w", r.EpisodeID, err)
	}
	return request.Snapshot.Facts, request.Snapshot.Evidence, nil
}

func (r *Request) BindExecutor(name, version, section string, document any) error {
	var request map[string]any
	if err := json.Unmarshal(r.RequestJSON, &request); err != nil {
		return fmt.Errorf("decode the request of episode %s: %w", r.EpisodeID, err)
	}
	if _, taken := request[section]; taken {
		return fmt.Errorf("the request of episode %s already has a %s section", r.EpisodeID, section)
	}
	request[section] = document
	bound, err := canonicaljson.Marshal(request)
	if err != nil {
		return fmt.Errorf("canonicalize the request of episode %s: %w", r.EpisodeID, err)
	}
	r.RequestJSON, r.ExecutorName, r.ExecutorVersion = bound, name, version
	return nil
}

func (r *Request) DispatchInShadow() {
	r.DispatchPolicy = spec.DispatchShadow
}
