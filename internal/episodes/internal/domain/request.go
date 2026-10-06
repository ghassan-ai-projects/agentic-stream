package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// BindAttemptIdentity binds a worker attempt identity into a persisted
// request document, keeping it canonical.
func BindAttemptIdentity(raw []byte, attemptID string, fence int64) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	document["attempt_id"] = attemptID
	document["fence"] = fence
	bound, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize request json: %w", err)
	}
	return bound, nil
}
