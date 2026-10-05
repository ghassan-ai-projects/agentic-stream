package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// BindAttemptIdentity binds a worker identity into a persisted request
// document, keeping it canonical.
func BindAttemptIdentity(raw []byte, identity episodeledger.Identity) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	document["attempt_id"] = identity.AttemptID
	document["fence"] = identity.Fence
	bound, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize request json: %w", err)
	}
	return bound, nil
}
