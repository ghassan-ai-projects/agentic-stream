package domain

import (
	"encoding/json"
	"fmt"
	"time"

	runtimeDuration "github.com/ghassan-ai-projects/agentic-stream/internal/duration"
)

// ParseWallTimeBudget validates and returns the durable wall-time budget
// carried by a canonical episode request document.
func ParseWallTimeBudget(requestJSON []byte) (time.Duration, error) {
	wallTime, err := requestWallTime(requestJSON)
	if err != nil {
		return 0, err
	}
	if wallTime == "" {
		return 0, nil
	}
	duration, err := runtimeDuration.Parse(wallTime)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
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
