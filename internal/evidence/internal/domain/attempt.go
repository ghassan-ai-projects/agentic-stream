package domain

import (
	"fmt"
)

// EpisodeState is the current durable episode binding.
type EpisodeState struct {
	Lifecycle, AttemptID string
	Fence                int64
}

// CheckLiveEpisode refuses stale or terminal call bindings before loading the attempt.
func CheckLiveEpisode(state EpisodeState, call Call) error {
	if state.AttemptID != call.AttemptID || state.Fence != call.Fence || state.Lifecycle == "concluded" || state.Lifecycle == "closed" || state.Lifecycle == "superseded" || state.Lifecycle == "expired" || state.Lifecycle == "abandoned" {
		return fmt.Errorf("evidence attempt is stale")
	}
	return nil
}

// CheckLiveAttempt refuses a terminal attempt at reservation.
func CheckLiveAttempt(status string) error {
	if status != "dispatched" && status != "running" {
		return fmt.Errorf("evidence attempt is terminal")
	}
	return nil
}

// CheckCompletionEpisode requires the original running binding at completion.
func CheckCompletionEpisode(state EpisodeState, key ReservationKey) error {
	if state.Lifecycle != "running" || state.AttemptID != key.AttemptID || state.Fence != key.Fence {
		return fmt.Errorf("evidence attempt is no longer current")
	}
	return nil
}

// CheckCompletionAttempt requires an active attempt at completion.
func CheckCompletionAttempt(status string) error {
	if status != "dispatched" && status != "running" {
		return fmt.Errorf("evidence attempt is no longer active")
	}
	return nil
}
