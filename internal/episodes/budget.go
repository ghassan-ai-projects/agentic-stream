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
	return r.parseWallTimeBudget()
}

func (r *Request) parseWallTimeBudget() (time.Duration, error) {
	wallTime, err := requestWallTime(r.RequestJSON)
	if err != nil {
		return 0, err
	}
	if wallTime == "" {
		r.wallTimeValidated = true
		return 0, nil
	}
	duration, err := runtimeDuration.Parse(wallTime)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
	r.wallTime = duration
	r.wallTimeValidated = true
	return duration, nil
}

func requestWallTime(raw []byte) (string, error) {
	var payload struct {
		Budget struct {
			WallTime string `json:"wall_time"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("decode episode budget: %w", err)
	}
	return payload.Budget.WallTime, nil
}
