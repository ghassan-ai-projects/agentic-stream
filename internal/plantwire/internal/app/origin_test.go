package app_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func commandingHostOn(t *testing.T, devices deviceAuthority, settings domain.Settings) runningHost {
	t.Helper()
	config := commandingConfig(t, devices)
	config.Settings, config.KindsDir, config.SitePath = settings, plantwirefake.KindsDir(t, "."), siteFile(t, pumpSite())
	return startHostWith(t, config, nil)
}

func hardwareNode(t *testing.T, host runningHost, session string) *plantwirefake.Node {
	t.Helper()
	node := host.node(t, session)
	birth := plantwirefake.Fixture(t, ".", "NBIRTH.json")
	birth["environment"] = domain.EnvironmentHardware
	for _, board := range birth["boards"].([]any) {
		board.(map[string]any)["environment"] = domain.EnvironmentHardware
	}
	node.Send(birth)
	if state := node.Expect("STATE"); state["online"] != true {
		t.Fatalf("handshake STATE = %v, want the hardware node accepted", state)
	}
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	settled(t, node)
	return node
}

func TestAHardwareDeviceIsCommandedWhenItsBoardIsNamed(t *testing.T) {
	t.Parallel()
	host := commandingHostOn(t, newDeviceAuthority(t), hardwareHost("board-01"))
	node := hardwareNode(t, host, "s-1")
	succeed(t, host, node, "c-1")
}

func TestAHardwareDeviceOfABoardNotNamedIsRefusedVisiblyAndNothingIsSent(t *testing.T) {
	t.Parallel()
	host := commandingHostOn(t, newDeviceAuthority(t), hardwareHost("board-02"))
	node := hardwareNode(t, host, "s-1")
	got := answered(t, dispatch(host, pumpCommand("c-1")))
	if !errors.Is(got.err, domain.ErrOriginNotAuthorized) {
		t.Fatalf("a command to an unnamed hardware board = %v, want ErrOriginNotAuthorized", got.err)
	}
	refusal := got.effect.ProviderResult["plant_wire"].(map[string]any)
	if refusal["refused"] != domain.RefusalOriginNotAuthorized || refusal["board"] != "board-01" || refusal["environment"] != domain.EnvironmentHardware {
		t.Fatalf("provider result = %v, want origin_not_authorized naming board-01", refusal)
	}
	assertNothingSent(t, node)
}
