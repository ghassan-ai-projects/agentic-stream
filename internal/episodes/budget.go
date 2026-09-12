package episodes

import (
	"encoding/json"
	"fmt"
	"time"

	runtimeDuration "github.com/ghassan-ai-projects/agentic-stream/internal/duration"
)

// WallTimeBudget validates and returns the durable wall-time budget once at
// the episode boundary. The typed value is then shared by the runner deadline
// gate and both executor adapters.
func (r *Request) WallTimeBudget() (time.Duration, error) {
	if r == nil {
		return 0, fmt.Errorf("episode request is required")
	}
	if r.wallTimeValidated {
		return r.wallTime, nil
	}
	var payload struct {
		Budget struct {
			WallTime string `json:"wall_time"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(r.RequestJSON, &payload); err != nil {
		return 0, fmt.Errorf("decode episode budget: %w", err)
	}
	if payload.Budget.WallTime == "" {
		r.wallTimeValidated = true
		return 0, nil
	}
	duration, err := runtimeDuration.Parse(payload.Budget.WallTime)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid wall_time budget %q", payload.Budget.WallTime)
	}
	r.wallTime = duration
	r.wallTimeValidated = true
	return duration, nil
}

func formatAcceptedAt(value time.Time) string {
	// accepted_at is ordered as SQLite TEXT. Fixed-width nanoseconds keep the
	// durable lexical order identical to chronological order across writers.
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
