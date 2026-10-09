package app_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var unknownAfterCrash = ledger{Command: "reconciling", Outbox: "failed", Outcome: "unknown", Reconciliation: "required", Verification: "awaiting", Outcomes: 1}

type countingObserver struct{ expiries int }

func (o *countingObserver) ObserveLeaseExpiry() { o.expiries++ }

func leaseAsCrashedWorker(t *testing.T, db *storage.DB, commandID, leaseUntil string) {
	t.Helper()
	execute(t, db, "UPDATE commands SET status = 'dispatching' WHERE command_id = ?", commandID)
	execute(t, db, "UPDATE outbox SET status = 'leased', lease_owner = 'crashed', lease_until = ?, attempt_count = 1 WHERE aggregate_id = ?", leaseUntil, commandID)
}

func TestACrashBetweenTheEffectAndItsOutcomeIsNeverDispatchedAgain(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	clock := sources.NewVirtual(fixtureNow)
	effector := succeeds()
	execute(t, db, `CREATE TRIGGER crash BEFORE INSERT ON outcomes BEGIN SELECT RAISE(ABORT, 'process died'); END`)

	crashed := newDispatcher(t, db, effector, withClock(clock), withName("crashed"))
	if _, err := crashed.DispatchOnce(t.Context()); err == nil || !strings.Contains(err.Error(), "process died") {
		t.Fatalf("err = %v, want the simulated crash while recording the outcome", err)
	}
	if got := readLedger(t, db, commandID); effector.calls != 1 || got.Command != "dispatching" || got.Outbox != "leased" || got.Outcomes != 0 {
		t.Fatalf("after the crash: effector calls = %d, ledger = %+v; want one effect with nothing recorded", effector.calls, got)
	}

	execute(t, db, "DROP TRIGGER crash")
	clock.Advance(2 * time.Minute)
	restarted := newDispatcher(t, db, effector, withClock(clock), withName("restarted"))
	if dispatchOnce(t, restarted) {
		t.Fatal("the restarted dispatcher reported a live dispatch of a command that may already have run")
	}
	if got := readLedger(t, db, commandID); effector.calls != 1 || got != unknownAfterCrash {
		t.Fatalf("after recovery: effector calls = %d, ledger = %+v; want one effect in total and %+v", effector.calls, got, unknownAfterCrash)
	}
	if dispatchOnce(t, restarted) || effector.calls != 1 {
		t.Fatalf("an unknown outcome was dispatched again: effector calls = %d", effector.calls)
	}
}

func TestALeaseThatCannotProveItIsLiveBecomesAnUnknownOutcomeWithoutAnEffectorCall(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"expired":                  kernel.FormatTime(fixtureNow.Add(-2 * time.Minute)),
		"offset in the far future": "2999-01-01T00:00:00.000000000+02:00",
		"impossible digits":        "9999-99-99T99:99:99.999999999Z",
		"empty":                    "",
	}
	for name, leaseUntil := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			leaseAsCrashedWorker(t, db, commandID, leaseUntil)
			effector, observer := succeeds(), &countingObserver{}
			if dispatchOnce(t, newDispatcher(t, db, effector, withObserver(observer))) {
				t.Fatal("an abandoned lease was reported as a live dispatch")
			}
			if got := readLedger(t, db, commandID); effector.calls != 0 || got != unknownAfterCrash || observer.expiries == 0 {
				t.Fatalf("effector calls = %d, expiries observed = %d, ledger = %+v; want no call, an observed expiry and %+v", effector.calls, observer.expiries, got, unknownAfterCrash)
			}
			assertTenantScopedLifecycleNotices(t, db)
		})
	}
}

func assertTenantScopedLifecycleNotices(t *testing.T, db *storage.DB) {
	t.Helper()
	for _, eventType := range []string{notify.CommandDispatched{}.EventType(), notify.OutcomeRecorded{}.EventType()} {
		event := readNotification(t, db, eventType)
		data, ok := event.Data.(map[string]any)
		if event.TenantID != "tenant" || event.Source != notify.SourceForTenant("tenant") || !ok || data["tenant_id"] != "tenant" || data["intent_id"] != "int-action" {
			t.Fatalf("%s = tenant %q source %q data %v; want a tenant-scoped event naming the intent", eventType, event.TenantID, event.Source, event.Data)
		}
	}
}

func TestAProviderResultThatArrivesAfterTheLeaseExpiredIsRecordedAsUnknown(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	clock := sources.NewVirtual(fixtureNow)
	effector, observer := succeeds(), &countingObserver{}
	effector.during = func() { clock.Advance(2 * time.Minute) }
	dispatchOnce(t, newDispatcher(t, db, effector, withClock(clock), withObserver(observer)))
	if got := readLedger(t, db, commandID); got != unknownAfterCrash || observer.expiries != 1 {
		t.Fatalf("ledger = %+v, expiries observed = %d; a success reported after the lease expired must be %+v", got, observer.expiries, unknownAfterCrash)
	}
}

func TestAProviderResultFromADispatcherThatLostItsLeaseIsDropped(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	effector := succeeds()
	effector.during = func() {
		execute(t, db, "UPDATE outbox SET lease_owner = 'another-dispatcher/lease' WHERE aggregate_id = ?", commandID)
	}
	dispatchOnce(t, newDispatcher(t, db, effector))
	if got := readLedger(t, db, commandID); got.Command != "dispatching" || got.Outbox != "leased" || got.Outcomes != 0 {
		t.Fatalf("ledger = %+v, want the late result dropped and the command left to its new lease holder", got)
	}
}

func TestALeaseThatExpiresBeforeRevalidationIsRecordedAsUnknownWithoutAnEffect(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	clock := sources.NewVirtual(fixtureNow)
	checks := 0
	owner := func(context.Context, *sql.Tx, string) error {
		if checks++; checks == 2 {
			clock.Advance(2 * time.Minute)
		}
		return nil
	}
	effector := succeeds()
	dispatchOnce(t, newDispatcher(t, db, effector, withClock(clock), withOwnerCheck(owner)))
	if got := readLedger(t, db, commandID); effector.calls != 0 || got != unknownAfterCrash {
		t.Fatalf("effector calls = %d, ledger = %+v; want no effect and %+v", effector.calls, got, unknownAfterCrash)
	}
}
