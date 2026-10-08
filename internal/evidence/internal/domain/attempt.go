package domain

import (
	"fmt"
)

// EpisodeState is the current durable episode binding.
type EpisodeState struct {
	Current         bool
	Closed, Running bool
}

// CheckLiveEpisode refuses stale or terminal call bindings before loading the attempt.
func CheckLiveEpisode(state EpisodeState) error {
	if !state.Current || state.Closed {
		return fmt.Errorf("evidence attempt is stale")
	}
	return nil
}

// CheckLiveAttempt refuses a terminal attempt at reservation.
func CheckLiveAttempt(inFlight bool) error {
	if !inFlight {
		return fmt.Errorf("evidence attempt is terminal")
	}
	return nil
}

// CheckCompletionEpisode requires the original running binding at completion.
func CheckCompletionEpisode(state EpisodeState) error {
	if !state.Running || !state.Current {
		return fmt.Errorf("evidence attempt is no longer current")
	}
	return nil
}

// CheckCompletionAttempt requires an active attempt at completion.
func CheckCompletionAttempt(inFlight bool) error {
	if !inFlight {
		return fmt.Errorf("evidence attempt is no longer active")
	}
	return nil
}
