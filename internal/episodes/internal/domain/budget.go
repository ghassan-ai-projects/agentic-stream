package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
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
	duration, err := spec.ParseDuration(wallTime)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
	return duration, nil
}

func requestWallTime(raw []byte) (string, error) {
	budget, err := decodeBudget[struct {
		WallTime string `json:"wall_time"`
	}](raw, "decode episode budget")
	return budget.WallTime, err
}

func decodeBudget[T any](raw []byte, failure string) (T, error) {
	var payload struct {
		Budget T `json:"budget"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload.Budget, fmt.Errorf("%s: %w", failure, err)
	}
	return payload.Budget, nil
}
