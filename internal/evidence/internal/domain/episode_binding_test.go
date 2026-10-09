package domain

import "testing"

func TestEpisodeBindingChecksRefuseStaleAndTerminalCalls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		state          EpisodeState
		liveAccepted   bool
		finishAccepted bool
	}{
		{"current and running", EpisodeState{Current: true, Running: true}, true, true},
		{"current and admitted", EpisodeState{Current: true}, true, false},
		{"current and closed", EpisodeState{Current: true, Closed: true}, false, false},
		{"stale and running", EpisodeState{Running: true}, false, false},
		{"stale and closed", EpisodeState{Closed: true}, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckLiveEpisode(test.state) == nil; got != test.liveAccepted {
				t.Fatalf("reservation accepted = %v, want %v", got, test.liveAccepted)
			}
			if got := CheckCompletionEpisode(test.state) == nil; got != test.finishAccepted {
				t.Fatalf("completion accepted = %v, want %v", got, test.finishAccepted)
			}
		})
	}
}

func TestAttemptChecksRequireAnInFlightAttempt(t *testing.T) {
	t.Parallel()
	if err := CheckLiveAttempt(true); err != nil {
		t.Fatalf("CheckLiveAttempt(in flight): %v", err)
	}
	if err := CheckCompletionAttempt(true); err != nil {
		t.Fatalf("CheckCompletionAttempt(in flight): %v", err)
	}
	requireErrorContaining(t, CheckLiveAttempt(false), "attempt is terminal")
	requireErrorContaining(t, CheckCompletionAttempt(false), "attempt is no longer active")
}
