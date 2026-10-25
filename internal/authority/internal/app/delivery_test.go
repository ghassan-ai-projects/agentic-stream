package app_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

const commandDigest = "sha256:abababababababababababababababababababababababababababababababab"

func fanBinding(commandID string, device domain.DeviceBoot) domain.CommandBinding {
	return domain.CommandBinding{CommandID: commandID, Target: "fan-01", Device: device, Owner: ownerOne, CommandDigest: commandDigest}
}

func (f *fixture) withAuthorityLoss(t *testing.T) *app.Service {
	t.Helper()
	lost := func(err error) bool { return errors.Is(err, control.ErrRuntimeOwnerBusy) }
	return app.New(app.Config{Store: store.New(f.db), Outcomes: actions.CountUnresolvedOutcomes, OwnerInstance: ownerOne.Instance,
		ClaimLease: 10 * time.Minute, Clock: f.clock,
		Fences: app.Fences{RuntimeOwner: f.runtime.Assert, EpochControl: (&control.EpochControl{DB: f.db}).AssertOrdinaryTx, AuthorityLost: lost}})
}

func TestClaimForDeliveryClaimsBindsAndAssertsInOrder(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.ClaimForDelivery(t.Context(), fanBinding("cmd-1", bootA)); err != nil {
		t.Fatalf("ClaimForDelivery = %v", err)
	}
	if err := f.service.AssertClaim(t.Context(), fanClaim); err != nil {
		t.Fatalf("the claim is not held after delivery: %v", err)
	}
	err := f.service.ClaimForDelivery(t.Context(), fanBinding("cmd-1", bootB))
	if !errors.Is(err, domain.ErrBindingRefused) || errors.Is(err, domain.ErrClaimRefused) {
		t.Fatalf("a conflicting binding = %v, want ErrBindingRefused", err)
	}
	if err := f.service.AssertClaim(t.Context(), fanClaim); !errors.Is(err, domain.ErrTargetClaimNotOwned) {
		t.Fatalf("the claim after a refused binding = %v, want it released", err)
	}
}

func TestClaimForDeliveryRefusesATargetAnotherOwnerHolds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.service.Claim(t.Context(), fanClaim); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.Release(t.Context(), ownerOne.Epoch); err != nil {
		t.Fatal(err)
	}
	ownerTwo := domain.Owner{Epoch: "epoch-2", Instance: "instance-2"}
	_, second := f.admit(t, ownerTwo, runtimeLease)
	binding := domain.CommandBinding{CommandID: "cmd-2", Target: "fan-01", Device: bootB, Owner: ownerTwo, CommandDigest: commandDigest}
	if err := second.ClaimForDelivery(t.Context(), binding); !errors.Is(err, domain.ErrClaimRefused) || !errors.Is(err, domain.ErrTargetClaimBusy) {
		t.Fatalf("ClaimForDelivery of a held target = %v, want ErrClaimRefused wrapping ErrTargetClaimBusy", err)
	}
}

func TestRequireReconciliationFallsBackToThePriorityPathAfterAuthorityLoss(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	opening := domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "plant wire outcome is unknown"}
	f.clock.Advance(runtimeLease)
	if err := f.service.RequireReconciliation(t.Context(), opening); !errors.Is(err, control.ErrRuntimeOwnerBusy) {
		t.Fatalf("RequireReconciliation without a loss classifier = %v, want the ordinary refusal", err)
	}
	lossAware := f.withAuthorityLoss(t)
	if err := lossAware.RequireReconciliation(t.Context(), opening); err != nil {
		t.Fatalf("RequireReconciliation after authority loss = %v", err)
	}
	if required, err := f.service.ReconciliationRequired(t.Context(), bootA.DeviceID); err != nil || !required {
		t.Fatalf("required = %v, %v; want the device blocked", required, err)
	}
}
