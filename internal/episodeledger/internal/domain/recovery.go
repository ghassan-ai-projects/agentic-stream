package domain

import (
	"encoding/json"
	"fmt"
)

// RecoveryReport describes active attempt state abandoned during a runtime
// restart.
type RecoveryReport struct {
	AbandonedAttempts int
	RequeuedEpisodes  int
	AbandonedEpisodes int
}

// UnfinishedAttempt is an active attempt owned by an older or missing epoch.
type UnfinishedAttempt struct {
	AttemptID  string
	EpisodeID  string
	Status     AttemptStatus
	OwnerEpoch string
}

// Canceling reports whether the attempt was being canceled; cancellation is a
// terminal decision, so its episode is abandoned rather than requeued.
func (u UnfinishedAttempt) Canceling() bool { return u.Status == AttemptCancelling }

// RecoveryTerminal is the terminal document of an abandoned attempt.
func (u UnfinishedAttempt) RecoveryTerminal() ([]byte, error) {
	terminal, err := json.Marshal(map[string]any{
		"status":               AttemptAbandoned,
		"reason":               "runtime_restart",
		"previous_owner_epoch": u.OwnerEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal recovery terminal: %w", err)
	}
	return terminal, nil
}

// IsRequeued reports whether a recovered episode stays live for another attempt.
func IsRequeued(lifecycle LifecycleStatus) bool {
	return lifecycle == LifecycleAdmitted || lifecycle == LifecycleRunning
}
