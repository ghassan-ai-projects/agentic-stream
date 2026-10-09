package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

func TestSettlingAnUnknownEpisodeFailsBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	err := settle(t, openStore(t), "missing", 0)
	if err == nil || !strings.Contains(err.Error(), "cost reservation missing is missing") {
		t.Fatalf("settlement of a missing reservation = %v", err)
	}
}

func TestReservationsRespectCeilingsAndSettleIdempotently(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	tenantLimit(t, persistence, "t", 10, false)
	if err := reserve(t, persistence, "ep1", "t", 6); err != nil {
		t.Fatal(err)
	}
	if err := reserve(t, persistence, "ep2", "t", 6); !errors.Is(err, domain.ErrCostReservationRejected) {
		t.Fatalf("over-ceiling reservation = %v", err)
	}
	if err := reserve(t, persistence, "ep3", "t", 0); !errors.Is(err, domain.ErrCostReservationRejected) {
		t.Fatalf("unestimated reservation under an active ceiling = %v", err)
	}
	if err := settle(t, persistence, "ep1", 4); err != nil {
		t.Fatal(err)
	}
	if err := settle(t, persistence, "ep1", 4); err != nil {
		t.Fatalf("repeat settlement = %v", err)
	}
	if err := settle(t, persistence, "ep1", 5); err == nil || !strings.Contains(err.Error(), "already settled with a different value") {
		t.Fatalf("settlement with a different value = %v", err)
	}
}

func TestAReservationNeedsTheGlobalLimitRowButNotATenantOne(t *testing.T) {
	t.Parallel()
	persistence, db := openStoreWithDB(t)
	if err := reserve(t, persistence, "ep1", "unlimited-tenant", 1); err != nil {
		t.Fatalf("a tenant without a limit row is unlimited: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "DELETE FROM cost_limits WHERE scope_key = ?", domain.GlobalScope); err != nil {
		t.Fatalf("delete the global limit row: %v", err)
	}
	err := reserve(t, persistence, "ep2", "unlimited-tenant", 1)
	if err == nil || !strings.Contains(err.Error(), "global cost limit is missing") {
		t.Fatalf("a missing global limit row must refuse the reservation: %v", err)
	}
}

func TestSettlementThatReachesACeilingTripsItsKillSwitch(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	tenantLimit(t, persistence, "t", 10, false)
	if err := reserve(t, persistence, "ep1", "t", 6); err != nil {
		t.Fatal(err)
	}
	if err := settle(t, persistence, "ep1", 10); err != nil {
		t.Fatal(err)
	}
	if _, kill, err := persistence.Autocommit().ReadLimit(t.Context(), domain.TenantScope("t")); err != nil || !kill {
		t.Fatalf("spend reaching the ceiling must trip the kill switch: kill=%v err=%v", kill, err)
	}
	if err := reserve(t, persistence, "ep2", "t", 1); !errors.Is(err, domain.ErrCostReservationRejected) {
		t.Fatalf("reservation after the kill switch tripped = %v", err)
	}
}

func TestApplyingCeilingsMergesTheGlobalLimitAndLeavesUnsetValuesAlone(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	setLimit(t, persistence, domain.GlobalScope, "", 100, true)
	tenantLimit(t, persistence, "t", 10, true)
	global, tenant, killOff := uint64(30), uint64(25), false
	apply := func(ceilings domain.CostCeilings) error {
		return persistence.WithTx(t.Context(), func(tx *store.Tx) error {
			return ApplyCeilings(t.Context(), tx, ceilings, "t", costTime)
		})
	}
	if err := apply(domain.CostCeilings{Tenant: &tenant}); err != nil {
		t.Fatal(err)
	}
	assertLimit(t, persistence, domain.GlobalScope, 100, true)
	assertLimit(t, persistence, domain.TenantScope("t"), 25, false)
	if err := apply(domain.CostCeilings{Global: &global, KillSwitch: &killOff}); err != nil {
		t.Fatal(err)
	}
	assertLimit(t, persistence, domain.GlobalScope, 30, false)
	assertLimit(t, persistence, domain.TenantScope("t"), 25, false)
}

func assertLimit(t *testing.T, persistence store.Store, scope string, wantMax int64, wantKill bool) {
	t.Helper()
	maxMicro, kill, err := persistence.Autocommit().ReadLimit(t.Context(), scope)
	if err != nil || maxMicro != wantMax || kill != wantKill {
		t.Fatalf("%s limit = %d kill=%v err=%v, want %d kill=%v", scope, maxMicro, kill, err, wantMax, wantKill)
	}
}
