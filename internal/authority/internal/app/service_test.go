package app_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

func TestOperationsRefuseAnotherOwnerInstance(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	foreign := domain.Owner{Epoch: "epoch-1", Instance: "instance-2"}
	claim := fanClaim
	claim.Owner = foreign
	checks := map[string]error{
		"claim":         f.service.Claim(t.Context(), claim),
		"safe stop":     f.service.RecordSafeStop(t.Context(), claim, domain.SafeStopRequested, nil),
		"open":          f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: foreign, Reason: "reason"}),
		"bind command":  f.service.BindCommand(t.Context(), domain.CommandBinding{CommandID: "c", Target: "fan-01", Device: bootA, Owner: foreign}),
		"partial owner": f.service.Claim(t.Context(), domain.TargetClaim{Target: "fan-01", Device: bootA, Owner: domain.Owner{Instance: "instance-1"}}),
	}
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s accepted a foreign or partial owner", name)
		}
	}
	if _, err := f.service.RecordDeviceState(t.Context(), foreign, map[string]any{"device_id": "d", "boot_id": "b"}); err == nil {
		t.Error("device state accepted a foreign owner")
	}
}

func TestAssertRuntimeFollowsTheRuntimeLease(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.AssertRuntime(t.Context(), ownerOne.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := f.service.AssertRuntime(t.Context(), ""); err == nil {
		t.Fatal("empty epoch was admitted")
	}
	f.clock.Advance(runtimeLease)
	if err := f.service.AssertRuntime(t.Context(), ownerOne.Epoch); !errors.Is(err, control.ErrRuntimeOwnerBusy) {
		t.Fatalf("expired runtime lease = %v", err)
	}
}
