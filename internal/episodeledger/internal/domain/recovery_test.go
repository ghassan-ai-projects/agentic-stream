package domain

import "testing"

func TestRecoveryTerminalNamesTheRestartAndThePreviousOwner(t *testing.T) {
	t.Parallel()
	item := UnfinishedAttempt{AttemptID: "a", EpisodeID: "e", Status: AttemptRunning, OwnerEpoch: "old"}
	terminal, err := item.RecoveryTerminal()
	want := `{"previous_owner_epoch":"old","reason":"runtime_restart","status":"abandoned"}`
	if err != nil || string(terminal) != want {
		t.Fatalf("terminal = %s err=%v, want %s", terminal, err, want)
	}
}

func TestOnlyACancellingAttemptAbandonsItsEpisodeOnRecovery(t *testing.T) {
	t.Parallel()
	for _, status := range attemptStatuses {
		if got, want := (UnfinishedAttempt{Status: status}).Canceling(), status == AttemptCancelling; got != want {
			t.Errorf("%s: Canceling = %v, want %v", status, got, want)
		}
	}
}

func TestARecoveredEpisodeIsRequeuedOnlyWhileLive(t *testing.T) {
	t.Parallel()
	for _, status := range lifecycleStatuses {
		if got := IsRequeued(status); got != status.Live() {
			t.Errorf("IsRequeued(%s) = %v, want %v", status, got, status.Live())
		}
	}
}
