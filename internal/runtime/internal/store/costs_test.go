package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestCostConfigurationCommitsOnlyUnderALiveOwnerLease(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	clock := sources.NewVirtual(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	owner := &control.RuntimeOwner{DB: db, InstanceID: "owner", Lease: time.Minute, Now: clock.Now}
	if err := owner.Claim(t.Context(), "epoch"); err != nil {
		t.Fatal(err)
	}
	ceiling := uint64(100)
	cfg := CostConfiguration{DB: db, RuntimeOwner: owner.Assert, OwnerEpoch: "epoch", Clock: clock, TenantID: "tenant", Ceilings: control.CostCeilings{Global: &ceiling}}

	if err := ConfigureCostLimits(t.Context(), cfg); err != nil {
		t.Fatalf("configure under a live lease: %v", err)
	}
	var recorded int
	if err := db.QueryRowContext(t.Context(), "SELECT max_micro FROM cost_limits WHERE scope_key = 'global'").Scan(&recorded); err != nil || recorded != 100 {
		t.Fatalf("global ceiling = %d, %v; want 100", recorded, err)
	}
	clock.Advance(2 * time.Minute)
	if err := ConfigureCostLimits(t.Context(), cfg); err == nil {
		t.Fatal("an expired owner configured cost limits")
	}
	adapter := &PipelineStore{DB: db, RuntimeOwner: owner.Assert, OwnerEpoch: "epoch"}
	if err := adapter.AssertOwner(t.Context()); err == nil {
		t.Fatal("an expired owner passed the pipeline fence")
	}
}

func TestEmptyCostCeilingsTouchNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	countLimits := func() (limits int) {
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM cost_limits").Scan(&limits); err != nil {
			t.Fatal(err)
		}
		return limits
	}
	before := countLimits()
	if err := ConfigureCostLimits(t.Context(), CostConfiguration{DB: db, OwnerEpoch: "epoch", Clock: sources.Physical(), TenantID: "tenant"}); err != nil {
		t.Fatalf("no ceilings must not even consult the owner: %v", err)
	}
	if after := countLimits(); after != before {
		t.Fatalf("cost limits changed from %d to %d", before, after)
	}
}
