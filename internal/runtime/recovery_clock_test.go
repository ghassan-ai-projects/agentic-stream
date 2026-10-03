package runtime

import (
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

func TestRecoveryWithoutOverrideUsesClaimTimestamp(t *testing.T) {
	db, now := seedRecoveryState(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-new", Lease: time.Minute, Now: func() time.Time { return now }}
	ledger := &evidence.Ledger{DB: db, LeaseOwner: "instance-new", RuntimeEpoch: "epoch-new", Lease: time.Minute}
	coordinator := &RecoveryCoordinator{Owner: owner, Ledger: ledger, Epoch: "epoch-new"}
	if _, err := coordinator.ClaimAndRecover(t.Context()); err != nil {
		t.Fatal(err)
	}
	var attemptEnded, evidenceCompleted string
	if err := db.QueryRowContext(t.Context(), "SELECT ended_at FROM episode_attempts WHERE attempt_id = 'attempt-old'").Scan(&attemptEnded); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT completed_at FROM evidence_call_ledger WHERE call_id = 'call-old'").Scan(&evidenceCompleted); err != nil {
		t.Fatal(err)
	}
	want := now.UTC().Format(time.RFC3339Nano)
	if attemptEnded != want || evidenceCompleted != want {
		t.Fatalf("recovery times = %q, %q; want claim time %q", attemptEnded, evidenceCompleted, want)
	}
}
