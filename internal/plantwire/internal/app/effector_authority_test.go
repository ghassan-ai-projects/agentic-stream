package app_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func succeed(t *testing.T, host runningHost, node *plantwirefake.Node, id string) {
	t.Helper()
	done := dispatch(host, pumpCommand(id))
	node.Expect("DCMD")
	node.Send(receipt(id, "accepted", ""))
	node.Send(result(id, "succeeded"))
	if got := answered(t, done); got.err != nil {
		t.Fatalf("%s = %v, want it pending verification", id, got.err)
	}
}

func assertNothingSent(t *testing.T, node *plantwirefake.Node) {
	t.Helper()
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": birthNS})
	node.Expect("HSYNC")
}

func TestHostClaimsAndBindsEachCommandAndReleasesItsClaimsWithTheSession(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	succeed(t, host, node, "c-1")
	if bound := devices.count(t, "SELECT COUNT(*) FROM device_command_bindings WHERE command_id = 'c-1'"); bound != 1 {
		t.Fatalf("bindings of c-1 = %d, want 1", bound)
	}
	if err := devices.service.AssertClaim(t.Context(), devices.pumpClaim()); err != nil {
		t.Fatalf("the target claim during the session: %v", err)
	}
	death(node, 0)
	if !node.Closed() {
		t.Fatal("the host kept the session after the node's death")
	}
	if err := devices.service.AssertClaim(t.Context(), devices.pumpClaim()); !errors.Is(err, authority.ErrTargetClaimNotOwned) {
		t.Fatalf("the target claim after the session = %v, want it released", err)
	}
}

func TestHostSendsNothingToADeviceWhoseLastOutcomeIsUnknown(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	done := dispatch(host, pumpCommand("c-1"))
	node.Expect("DCMD")
	node.Send(receipt("c-1", "accepted", ""))
	node.Send(result("c-1", "unknown"))
	if got := answered(t, done); !actionport.IsUnknownOutcome(got.err) {
		t.Fatalf("c-1 = %v, want an unknown outcome", got.err)
	}
	if got := answered(t, dispatch(host, pumpCommand("c-2"))); !errors.Is(got.err, authority.ErrReconciliationRequired) {
		t.Fatalf("c-2 after an unknown outcome = %v, want ErrReconciliationRequired", got.err)
	}
	assertNothingSent(t, node)
}

func TestHostSendsNothingToACommandedDeviceThatRebooted(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	succeed(t, host, node, "c-1")
	node.Send(plantwirefake.Fixture(t, ".", "DDEATH.json"))
	reborn := plantwirefake.Fixture(t, ".", "DBIRTH.json")
	reborn["boot_id"] = "board-01-boot-2"
	node.Send(reborn)
	host.event(t)
	if got := answered(t, dispatch(host, pumpCommand("c-2"))); !errors.Is(got.err, authority.ErrReconciliationRequired) {
		t.Fatalf("c-2 after a reboot = %v, want ErrReconciliationRequired", got.err)
	}
	assertNothingSent(t, node)
}

func TestHostSendsNothingWhenTheRuntimeOwnerLostItsAuthority(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	if err := devices.owner.Release(t.Context(), ownerEpoch); err != nil {
		t.Fatal(err)
	}
	if got := answered(t, dispatch(host, pumpCommand("c-1"))); got.err == nil || actionport.IsUnknownOutcome(got.err) {
		t.Fatalf("a command without authority = %v, want a known refusal", got.err)
	}
	assertNothingSent(t, node)
}

func TestNewRefusesACommandingHostWithoutTheDeviceAuthority(t *testing.T) {
	t.Parallel()
	config := commandingConfig(t, newDeviceAuthority(t))
	config.Listener, config.Settings, config.KindsDir = listener(t), simulatedHost, plantwirefake.KindsDir(t, ".")
	config.Authority = nil
	if _, err := app.New(config); !errors.Is(err, app.ErrAuthorityRequired) {
		t.Fatalf("New(a catalog without authority) = %v, want ErrAuthorityRequired", err)
	}
	config.Authority, config.OwnerEpoch = newDeviceAuthority(t).service, ""
	if _, err := app.New(config); !errors.Is(err, app.ErrAuthorityRequired) {
		t.Fatalf("New(a catalog without an owner epoch) = %v, want ErrAuthorityRequired", err)
	}
}
