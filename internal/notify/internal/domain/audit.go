package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// AuditDetails is the canonical details document of a notification audit.
func AuditDetails(requested, oldest int64) ([]byte, error) {
	details, err := canonicaljson.Marshal(map[string]any{"requested_cursor": requested, "oldest_cursor": oldest})
	if err != nil {
		return nil, fmt.Errorf("canonicalize notification audit details: %w", err)
	}
	return details, nil
}
