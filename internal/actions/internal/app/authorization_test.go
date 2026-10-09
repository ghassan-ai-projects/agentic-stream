package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var refusedCommand = ledger{Command: "failed", Outbox: "failed", Outcome: "failed", Reconciliation: "not_required", Verification: "awaiting", Outcomes: 1}

func approveWithPolicy(t *testing.T, db *storage.DB, digest string) {
	t.Helper()
	execute(t, db, `
		INSERT INTO policy_evaluations (evaluation_id, intent_id, decision_id, policy_version, policy_digest,
			intent_sha256, decision_sha256, result, reason, situation_version, evaluated_at)
		VALUES ('eval-1', 'int-action', 'dec-action', 'v1', ?, zeroblob(32), zeroblob(32), 'approved', 'ok', 1, ?)`,
		digest, kernel.FormatTime(fixtureNow))
}

func approve(t *testing.T, db *storage.DB, expiresAt time.Time) {
	t.Helper()
	execute(t, db, "UPDATE intents SET requires_approval = 1")
	execute(t, db, `
		INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, decided_at, approval_json)
		VALUES ('apr-1', 'int-action', 'approved', ?, ?, ?, X'7B7D')`,
		kernel.FormatTime(fixtureNow.Add(-time.Hour)), kernel.FormatTime(expiresAt), kernel.FormatTime(fixtureNow.Add(-time.Minute)))
}

func sqlStep(statement string) func(*testing.T, *storage.DB) {
	return func(t *testing.T, db *storage.DB) {
		t.Helper()
		execute(t, db, statement)
	}
}

func approvedUntil(expiresAt time.Time) func(*testing.T, *storage.DB) {
	return func(t *testing.T, db *storage.DB) {
		t.Helper()
		approve(t, db, expiresAt)
	}
}

func policyApproving(digest string) func(*testing.T, *storage.DB) {
	return func(t *testing.T, db *storage.DB) {
		t.Helper()
		approveWithPolicy(t, db, digest)
	}
}

func TestAnIntentIsRevalidatedAgainstCurrentStateJustBeforeTheEffector(t *testing.T) {
	t.Parallel()
	staleDigest := "sha256:" + strings.Repeat("f", 64)
	cases := []struct {
		name       string
		spec       fixtureSpec
		clockAfter time.Duration
		prepare    func(*testing.T, *storage.DB)
		dispatches bool
	}{
		{name: "current and approved", dispatches: true},
		{name: "policy no longer approves the intent", prepare: sqlStep("UPDATE intents SET policy_status = 'pending'")},
		{name: "decision was rejected", prepare: sqlStep("UPDATE decisions SET validation_status = 'rejected'")},
		{name: "a newer material Situation version exists", prepare: sqlStep("UPDATE situations SET last_material_version = 2")},
		{name: "the episode has not produced its decision", prepare: sqlStep("UPDATE episodes SET lifecycle_status = 'running'")},
		{name: "the intent expired", clockAfter: 2 * time.Hour},
		{name: "approval is required and absent", prepare: sqlStep("UPDATE intents SET requires_approval = 1")},
		{name: "approval is required and unexpired", prepare: approvedUntil(fixtureNow.Add(30 * time.Minute)), dispatches: true},
		{name: "approval expired", prepare: approvedUntil(fixtureNow.Add(-time.Second))},
		{name: "command policy digest matches the approving evaluation", spec: fixtureSpec{policyDigest: zeroDigest},
			prepare: policyApproving(zeroDigest), dispatches: true},
		{name: "command policy digest is stale", spec: fixtureSpec{policyDigest: zeroDigest},
			prepare: policyApproving(staleDigest)},
		{name: "command policy digest has no approving evaluation", spec: fixtureSpec{policyDigest: zeroDigest}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openFixture(t, tc.spec)
			if tc.prepare != nil {
				tc.prepare(t, db)
			}
			effector := succeeds()
			clock := sources.NewVirtual(fixtureNow.Add(tc.clockAfter))
			if !dispatchOnce(t, newDispatcher(t, db, effector, withClock(clock))) {
				t.Fatal("the command was not leased")
			}
			wantCalls, want := 0, refusedCommand
			if tc.dispatches {
				wantCalls, want = 1, ledger{Command: "succeeded", Outbox: "delivered", Outcome: "succeeded", Reconciliation: "observed", Verification: "observed", Outcomes: 1}
			}
			if got := readLedger(t, db, commandID); effector.calls != wantCalls || got != want {
				t.Fatalf("effector calls = %d, ledger = %+v; want %d calls and %+v", effector.calls, got, wantCalls, want)
			}
		})
	}
}

func TestATrippedInterlockFailsTheCommandBeforeTheEffector(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "maintenance stop", fixtureNow)
		return err
	}); err != nil {
		t.Fatalf("trip interlock: %v", err)
	}
	effector := succeeds()
	dispatchOnce(t, newDispatcher(t, db, effector))
	if got := readLedger(t, db, commandID); effector.calls != 0 || got != refusedCommand {
		t.Fatalf("effector calls = %d, ledger = %+v; want no call and %+v", effector.calls, got, refusedCommand)
	}
}

func TestTheInterlockIsCheckedAgainAtTheMomentTheEffectorAccepts(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	effector := &tripBeforeAcceptEffector{db: db}
	dispatchOnce(t, newDispatcher(t, db, effector))
	if got := readLedger(t, db, commandID); effector.calls != 0 || got != refusedCommand {
		t.Fatalf("effector accepted %d times, ledger = %+v; want no acceptance and %+v", effector.calls, got, refusedCommand)
	}
}

func TestADispatcherWithoutRuntimeOwnershipNeitherLeasesNorDispatches(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	lost := errors.New("ownership lost")
	effector := succeeds()
	dispatcher := newDispatcher(t, db, effector, withOwnerCheck(func(context.Context, *sql.Tx, string) error { return lost }))
	processed, err := dispatcher.DispatchOnce(t.Context())
	if !errors.Is(err, lost) || processed || effector.calls != 0 {
		t.Fatalf("processed = %v, calls = %d, err = %v; want the ownership error and no work", processed, effector.calls, err)
	}
	if got := readLedger(t, db, commandID); got.Command != "pending" || got.Outbox != "pending" || got.Outcomes != 0 {
		t.Fatalf("ledger = %+v, want the command untouched", got)
	}
}

func TestOwnershipLostAfterLeasingStopsTheDispatchBeforeTheEffector(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	lost := errors.New("ownership lost")
	checks := 0
	owner := func(context.Context, *sql.Tx, string) error {
		checks++
		if checks > 1 {
			return lost
		}
		return nil
	}
	effector := succeeds()
	if _, err := newDispatcher(t, db, effector, withOwnerCheck(owner)).DispatchOnce(t.Context()); !errors.Is(err, lost) || effector.calls != 0 {
		t.Fatalf("calls = %d, err = %v; want the ownership error and no effect", effector.calls, err)
	}
	if got := readLedger(t, db, commandID); got.Command != "dispatching" || got.Outbox != "leased" || got.Outcomes != 0 {
		t.Fatalf("ledger = %+v, want the command still leased and unrecorded", got)
	}
}
