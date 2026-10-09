package device_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

func openEmulatedGatewayEffector(t *testing.T) (context.Context, *device.GatewayEffector) {
	t.Helper()
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest catalog: %v", err)
	}
	socket := startEmulatedDevice(t, digest)
	authority := newDeviceAuthority(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	link, err := device.DialUDSTransport(ctx, socket)
	if err != nil {
		t.Fatalf("dial device gateway: %v", err)
	}
	effector, closeEffector, err := device.NewGatewayEffector(ctx, device.GatewayEffectorConfig{
		Transport: link, Catalog: catalog, AllowedFirmwareDigests: []string{firmwareDigest()},
		OwnerEpoch: "epoch-1", OwnerInstance: "instance-1", Authority: authority,
	})
	if err != nil {
		t.Fatalf("open gateway effector: %v", err)
	}
	t.Cleanup(func() { _ = closeEffector() })
	return ctx, effector
}

func indicatorCommand(commandID, idempotencyKey string) actionport.Command {
	return actionport.Command{
		CommandID: commandID, EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idempotencyKey, PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}
}

func TestGatewayEffectorDrivesAnEmulatedDeviceOverAUnixSocket(t *testing.T) {
	t.Parallel()
	ctx, effector := openEmulatedGatewayEffector(t)

	effect, err := effector.Dispatch(ctx, indicatorCommand("cmd-1", idemKey()))
	if err != nil || !effect.VerificationPending {
		t.Fatalf("dispatch over the socket: effect=%+v err=%v; an accepted command must leave verification pending", effect, err)
	}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	authorized, err := effector.WithTelemetry(nil).DispatchAuthorized(ctx, indicatorCommand("cmd-2", "sha256:"+strings.Repeat("e", 64)), allow)
	if err != nil || !authorized.VerificationPending {
		t.Fatalf("authorized dispatch = %+v, %v", authorized, err)
	}
	status, evidence, err := effector.VerifyDeviceCommand(ctx, indicatorCommand("cmd-1", idemKey()))
	if err != nil || status != "succeeded" || evidence["evidence_digest"] == nil {
		t.Fatalf("verification = %q, %v, %v; want the emulated output to confirm the command", status, evidence, err)
	}
	if _, err := effector.SafeStop(ctx, "fan-01"); err != nil {
		t.Fatalf("safe stop over the socket: %v", err)
	}
}

func TestFallbackEffectorsSimulateOrRefuseButNeverReachADevice(t *testing.T) {
	t.Parallel()
	command := actionport.Command{CommandID: "c", EffectorRoute: "notify", IdempotencyKey: idemKey()}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	effect, err := device.NewSimulatedEffector().DispatchAuthorized(t.Context(), command, allow)
	if err != nil || effect.ProviderResult["accepted"] != true {
		t.Fatalf("simulated effector = %+v, %v", effect, err)
	}
	_, err = device.NewFailClosedEffector(device.EffectProfilePhysical).DispatchAuthorized(t.Context(), command, allow)
	if err == nil || !strings.Contains(err.Error(), `physical effect profile has no effector for route "notify"`) {
		t.Fatalf("fail-closed effector error = %v", err)
	}
}

func TestNewGatewayEffectorRefusesAnIncompleteConfigurationWithoutOwningTheLink(t *testing.T) {
	t.Parallel()
	effector, closeEffector, err := device.NewGatewayEffector(t.Context(), device.GatewayEffectorConfig{Catalog: loadThermalCatalog(t)})
	if err == nil || !strings.Contains(err.Error(), "requires a device transport") || effector != nil || closeEffector != nil {
		t.Fatalf("effector=%v close=%v err=%v", effector, closeEffector != nil, err)
	}
}
