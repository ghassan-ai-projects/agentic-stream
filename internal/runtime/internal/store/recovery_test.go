package store

import (
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestRecoveryCoordinatorAtomicallyRecoversEpisodesAndEvidence(t *testing.T) {
	t.Parallel()
	db, now := seedRecoveryState(t)
	coordinator := newRecoveryCoordinator(t, db, now, true)

	report, err := coordinator.ClaimAndRecover(t.Context())
	if err != nil {
		t.Fatalf("claim and recover: %v", err)
	}
	if report.Episodes.AbandonedAttempts != 1 || report.Episodes.RequeuedEpisodes != 1 || report.InterruptedEvidence != 1 {
		t.Fatalf("recovery report = %+v", report)
	}
	if attempt, call := recoveryStatuses(t, db); attempt != "abandoned" || call != "interrupted" {
		t.Fatalf("recovered statuses attempt=%q evidence=%q; want abandoned and interrupted", attempt, call)
	}
}

func TestRecoveryWithoutAClockOverrideUsesTheClaimTimestamp(t *testing.T) {
	t.Parallel()
	db, now := seedRecoveryState(t)
	if _, err := newRecoveryCoordinator(t, db, now, false).ClaimAndRecover(t.Context()); err != nil {
		t.Fatal(err)
	}
	var attemptEnded, evidenceCompleted string
	if err := db.QueryRowContext(t.Context(), "SELECT ended_at FROM episode_attempts WHERE attempt_id = 'attempt-old'").Scan(&attemptEnded); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT completed_at FROM evidence_call_ledger WHERE call_id = 'call-old'").Scan(&evidenceCompleted); err != nil {
		t.Fatal(err)
	}
	if want := kernel.FormatTime(now.UTC()); attemptEnded != want || evidenceCompleted != want {
		t.Fatalf("recovery times = %q, %q; want claim time %q", attemptEnded, evidenceCompleted, want)
	}
}

func TestRecoveryRefusesAnIncompleteOrMismatchedConfiguration(t *testing.T) {
	t.Parallel()
	db, now := seedRecoveryState(t)
	complete := newRecoveryCoordinator(t, db, now, true)
	mismatched := *complete
	mismatched.Epoch = "another-epoch"
	noEpoch := *complete
	noEpoch.Epoch = ""
	noOwner := *complete
	noOwner.Owner = nil
	noLedger := *complete
	noLedger.Ledger = nil
	tests := map[string]struct {
		coordinator *RecoveryCoordinator
		want        string
	}{
		"nil":                                   {nil, "runtime recovery is not configured"},
		"no owner":                              {&noOwner, "runtime recovery is not configured"},
		"no ledger":                             {&noLedger, "runtime recovery is not configured"},
		"no epoch":                              {&noEpoch, "runtime recovery is not configured"},
		"ledger epoch differs from owner epoch": {&mismatched, "ledger runtime epoch does not match owner epoch"},
	}
	for name, tt := range tests {
		if _, err := tt.coordinator.ClaimAndRecover(t.Context()); err == nil || err.Error() != tt.want {
			t.Errorf("%s: recovery = %v, want %q", name, err, tt.want)
		}
	}
	if attempt, call := recoveryStatuses(t, db); attempt != "running" || call != "running" {
		t.Fatalf("a refused recovery changed state: attempt=%q evidence=%q", attempt, call)
	}
}

func TestAFailedClaimRecoversNothing(t *testing.T) {
	t.Parallel()
	db, now := seedRecoveryState(t)
	holder := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-holder", Lease: time.Hour, Now: func() time.Time { return now }}
	if err := holder.Claim(t.Context(), "epoch-holder"); err != nil {
		t.Fatal(err)
	}

	_, err := newRecoveryCoordinator(t, db, now, true).ClaimAndRecover(t.Context())

	if !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
		t.Fatalf("claim against a live owner = %v, want %v", err, runtimecontrol.ErrRuntimeOwnerBusy)
	}
	if attempt, call := recoveryStatuses(t, db); attempt != "running" || call != "running" {
		t.Fatalf("recovery ran without ownership: attempt=%q evidence=%q", attempt, call)
	}
}
