package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const slowPumpCatalog = `{"protocol_version": 1, "routes": {"set_pump_speed": {"operation": "set_speed", "target": "P1",
 "target_bindings": {"station-1": "P1"}, "selector_field": "speed", "expires_after_ms": 200000,
 "presets": {"half": {"pct": 50, "lease_ms": 5000}}, "bounds": {"pct": {"min": 0, "max": 100}}}}}`

func mixedHost(boards ...string) domain.Settings {
	return domain.Settings{Environment: domain.EnvironmentMixed, HardwareBoards: boards, PhysicalConsent: true, Epoch: epoch}
}

func dispatchWith(host runningHost, command actionport.Command, allow actionport.Authorization) <-chan dispatched {
	done := make(chan dispatched, 1)
	go func() {
		effect, err := host.host.DispatchAuthorized(context.Background(), command, allow)
		done <- dispatched{effect: effect, err: err}
	}()
	return done
}

func movedBirth(t *testing.T) map[string]any {
	t.Helper()
	birth := plantwirefake.Fixture(t, ".", "NBIRTH.json")
	birth["bd_seq"] = 1
	birth["environment"] = domain.EnvironmentMixed
	birth["boards"] = []any{
		map[string]any{"board": "board-01", "environment": domain.EnvironmentSimulated, "members": []any{}},
		map[string]any{"board": "hw-9", "environment": domain.EnvironmentHardware, "members": []any{"P1"}},
	}
	return birth
}

func movedDeviceBirth(t *testing.T) map[string]any {
	t.Helper()
	birth := plantwirefake.Fixture(t, ".", "DBIRTH.json")
	birth["board"] = "hw-9"
	return birth
}

func TestACommandWhoseDeviceMovesToAnotherBoardBeforeTheSendIsRefused(t *testing.T) {
	t.Parallel()
	host := commandingHostOn(t, newDeviceAuthority(t), mixedHost("board-01"))
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	settled(t, node)
	allow := actionport.Authorization{Check: func(context.Context) error {
		node.Send(movedBirth(t))
		node.Send(movedDeviceBirth(t))
		settled(t, node)
		return nil
	}}
	got := answered(t, dispatchWith(host, pumpCommand("c-1"), allow))
	if got.err == nil || !strings.Contains(got.err.Error(), domain.RefusalOriginNotAuthorized) {
		t.Fatalf("a command to a device that moved to another board = %v, want an origin_not_authorized refusal", got.err)
	}
	if strings.Contains(got.err.Error(), "hw-9") == false {
		t.Errorf("the refusal %v does not name the board the device moved to", got.err)
	}
	assertNothingSent(t, node)
}

func TestACommandToASimulatedDeviceIsSentWhileItsBoardIsNamed(t *testing.T) {
	t.Parallel()
	host := commandingHostOn(t, newDeviceAuthority(t), mixedHost("board-01"))
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	settled(t, node)
	succeed(t, host, node, "c-1")
}

func TestACommandHeldPastTheClaimLeaseKeepsItsAuthority(t *testing.T) {
	t.Parallel()
	host, node := slowHost(t)
	done := dispatch(host, pumpCommand("c-1"))
	node.Expect("DCMD")
	host.clock.Advance(90 * time.Second)
	node.Send(receipt("c-1", "accepted", ""))
	node.Send(result("c-1", "succeeded"))
	got := answered(t, done)
	if got.err != nil || !got.effect.VerificationPending {
		t.Fatalf("a command held past the claim lease gave %+v, %v; want it pending verification", got.effect, got.err)
	}
}

func TestALostAuthorityWhileACommandWaitsMakesItsOutcomeUnknown(t *testing.T) {
	t.Parallel()
	host, node, devices := slowHostWith(t)
	done := dispatch(host, pumpCommand("c-1"))
	node.Expect("DCMD")
	if err := devices.owner.Release(t.Context(), ownerEpoch); err != nil {
		t.Fatalf("release the runtime owner: %v", err)
	}
	host.clock.Advance(90 * time.Second)
	node.Send(receipt("c-1", "accepted", ""))
	got := answered(t, done)
	if !actionport.IsUnknownOutcome(got.err) {
		t.Fatalf("a command whose authority was lost = %v, want an unknown outcome", got.err)
	}
}

func TestACommandTheHostCannotEncodeIsNotSentAndEndsNoSession(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	broken := pumpCommand("c-1")
	broken.PolicyDigest = "not-a-digest"
	got := answered(t, dispatch(host, broken))
	if got.err == nil || !strings.Contains(got.err.Error(), "not sent") {
		t.Fatalf("a command the host cannot encode = %v, want it reported as not sent", got.err)
	}
	if outcome := got.effect.ProviderResult["plant_wire"].(map[string]any); outcome["outcome"] != domain.SettlementNotSent {
		t.Fatalf("provider result = %v, want the outcome not_sent", outcome)
	}
	settled(t, node)
}

func slowHost(t *testing.T) (runningHost, *plantwirefake.Node) {
	t.Helper()
	host, node, _ := slowHostWith(t)
	return host, node
}

func slowHostWith(t *testing.T) (runningHost, *plantwirefake.Node, deviceAuthority) {
	t.Helper()
	devices := newDeviceAuthority(t)
	catalog, err := device.LoadCapabilityCatalog([]byte(slowPumpCatalog))
	if err != nil {
		t.Fatal(err)
	}
	config := commandingConfig(t, devices)
	config.Catalog = catalog
	host := pumpHost(t, config, nil)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	host.event(t)
	return host, node, devices
}

func TestASiteDeviceTheBirthDoesNotCarryIsRefusedAtTheHandshake(t *testing.T) {
	t.Parallel()
	devices := newDeviceAuthority(t)
	config := commandingConfig(t, devices)
	site := pumpSite()
	site["devices"].(map[string]any)["P9"] = map[string]any{"kind": "pump", "entity": map[string]any{"type": "station", "id": "station-9"}}
	config.Settings, config.KindsDir = simulatedHost, plantwirefake.KindsDir(t, ".")
	config.SitePath = siteFile(t, site)
	host := startHostWith(t, config, nil)
	node := host.node(t, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
	state := node.Expect("STATE")
	if state["online"] != false || state["refusal"] != domain.RefusalInventoryMismatch {
		t.Fatalf("the handshake STATE = %v, want the session refused for a device the birth does not carry", state)
	}
}
