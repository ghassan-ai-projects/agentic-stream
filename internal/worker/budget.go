package worker

import (
	"fmt"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// ValidateBudget requires a worker request to carry a finite wall-time
// boundary. A worker is an external process, so model-call and byte counters
// cannot protect the runtime from a handler that stops emitting events.
func ValidateBudget(budget *runtimev1.EpisodeBudget) error {
	if budget == nil || budget.GetWallTime() == nil || !budget.GetWallTime().IsValid() {
		return fmt.Errorf("worker budget requires a valid wall_time")
	}
	if budget.GetWallTime().AsDuration() <= 0 {
		return fmt.Errorf("worker budget wall_time must be positive")
	}
	return nil
}
