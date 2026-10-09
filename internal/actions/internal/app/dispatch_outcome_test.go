package app_test

import (
	"context"
	"errors"
	"testing"
)

func TestADispatchRecordsItsOutcomeAndTheEffectIsNeverRepeated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		effector *scriptedEffector
		want     ledger
	}{
		{"success", succeeds(),
			ledger{Command: "succeeded", Outbox: "delivered", Outcome: "succeeded", Reconciliation: "observed", Verification: "observed", Outcomes: 1}},
		{"unknown outcome", failsWith(unknownOutcome),
			ledger{Command: "reconciling", Outbox: "failed", Outcome: "unknown", Reconciliation: "required", Verification: "awaiting", Outcomes: 1}},
		{"provider deadline is an unknown outcome", failsWith(context.DeadlineExceeded),
			ledger{Command: "reconciling", Outbox: "failed", Outcome: "unknown", Reconciliation: "required", Verification: "awaiting", Outcomes: 1}},
		{"known failure", failsWith(errors.New("provider rejected the command")),
			ledger{Command: "failed", Outbox: "failed", Outcome: "failed", Reconciliation: "not_required", Verification: "awaiting", Outcomes: 1}},
		{"accepted transport still needs verification", pendingVerification(),
			ledger{Command: "manual_review", Outbox: "delivered", Outcome: "reconcile_required", Reconciliation: "required", Verification: "awaiting", Outcomes: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			dispatcher := newDispatcher(t, db, tc.effector)
			if !dispatchOnce(t, dispatcher) {
				t.Fatal("the first dispatch found nothing to do")
			}
			if got := readLedger(t, db, commandID); got != tc.want {
				t.Fatalf("ledger = %+v, want %+v", got, tc.want)
			}
			if dispatchOnce(t, dispatcher) || tc.effector.calls != 1 {
				t.Fatalf("a second dispatch must find nothing; effector calls = %d, want 1", tc.effector.calls)
			}
		})
	}
}

func TestAnOutboxRowReplayedForAFinishedCommandIsClosedWithoutCallingTheEffector(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		commandState string
		wantOutbox   string
	}{
		{"succeeded command", "succeeded", "delivered"},
		{"command with an unknown outcome", "outcome_unknown", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			execute(t, db, "UPDATE commands SET status = ? WHERE command_id = ?", tc.commandState, commandID)
			effector := succeeds()
			dispatcher := newDispatcher(t, db, effector)
			if dispatchOnce(t, dispatcher) {
				t.Fatal("a replayed outbox row was reported as a live dispatch")
			}
			got := readLedger(t, db, commandID)
			if effector.calls != 0 || got.Command != tc.commandState || got.Outbox != tc.wantOutbox || got.Outcomes != 0 {
				t.Fatalf("effector calls = %d, ledger = %+v; want no call, command %q and outbox %q", effector.calls, got, tc.commandState, tc.wantOutbox)
			}
		})
	}
}

func TestATamperedCommandDocumentFailsTheCommandWithoutCallingTheEffector(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	execute(t, db, `UPDATE commands SET command_json = CAST(replace(CAST(command_json AS TEXT), 'motor/1', 'motor/9') AS BLOB) WHERE command_id = ?`, commandID)
	effector := succeeds()
	if dispatchOnce(t, newDispatcher(t, db, effector)) {
		t.Fatal("a tampered command was reported as a live dispatch")
	}
	got := readLedger(t, db, commandID)
	code := queryString(t, db, "SELECT last_error_code FROM outbox WHERE aggregate_id = ?", commandID)
	if effector.calls != 0 || got.Command != "failed" || got.Outbox != "failed" || code != "command_digest_mismatch" {
		t.Fatalf("effector calls = %d, ledger = %+v, error code = %q; want a failed command with command_digest_mismatch", effector.calls, got, code)
	}
}
