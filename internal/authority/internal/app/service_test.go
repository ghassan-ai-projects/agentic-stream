package app_test

import (
	"context"
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

func TestTheServiceReportsTheOwnerInstanceItWasConfiguredFor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if got := f.service.OwnerInstance(); got != ownerOne.Instance {
		t.Fatalf("OwnerInstance = %q, want %q", got, ownerOne.Instance)
	}
}

// A draining or killed epoch is the control plane stopping the owner: ordinary
// operations stop, while the priority path still makes the device safer.
func TestADrainingOrKilledEpochRefusesOrdinaryOperationsButNotThePriorityPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stop func(*control.EpochControl, context.Context, string) error
		want error
	}{
		{"draining", (*control.EpochControl).Drain, control.ErrEpochDraining},
		{"killed", (*control.EpochControl).Kill, control.ErrEpochKilled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if err := f.service.Claim(t.Context(), fanClaim); err != nil {
				t.Fatal(err)
			}
			if err := test.stop(&control.EpochControl{DB: f.db}, t.Context(), ownerOne.Epoch); err != nil {
				t.Fatal(err)
			}

			ordinary := map[string]error{
				"AssertRuntime": f.service.AssertRuntime(t.Context(), ownerOne.Epoch),
				"Claim":         f.service.Claim(t.Context(), fanClaim),
				"AssertClaim":   f.service.AssertClaim(t.Context(), fanClaim),
				"BindCommand":   f.service.BindCommand(t.Context(), domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: ownerOne}),
			}
			for operation, err := range ordinary {
				if !errors.Is(err, test.want) {
					t.Errorf("%s in a %s epoch = %v, want %v", operation, test.name, err, test.want)
				}
			}
			if err := f.service.RecordSafeStop(t.Context(), fanClaim, domain.SafeStopRequested, nil); err != nil {
				t.Errorf("safe stop in a %s epoch: %v", test.name, err)
			}
			if err := f.service.ReleaseClaim(t.Context(), fanClaim); err != nil {
				t.Errorf("release in a %s epoch: %v", test.name, err)
			}
		})
	}
}
