package episodes

import (
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
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
	duration, err := domain.ParseWallTimeBudget(r.RequestJSON)
	if err != nil {
		return 0, err
	}
	r.wallTime = duration
	r.wallTimeValidated = true
	return duration, nil
}
