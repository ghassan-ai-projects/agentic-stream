package device_test

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// devicePeer is an in-process device that speaks the wire contract over a UDS
// with the digests this session expects, so the gateway effector can complete a
// real handshake + governed command exchange without a cross-repo binary. It is
// the same behavior `streamsim device serve` provides.
func devicePeer(t *testing.T, conn net.Conn, capabilityDigest string) {
	t.Helper()
	defer func() { _ = conn.Close() }()
	write := func(doc map[string]any) bool {
		frame, err := canonicaljson.Marshal(doc)
		if err != nil {
			return false
		}
		_, err = conn.Write(append(frame, '\n'))
		return err == nil
	}
	state := goldenDeviceState()
	state["capability_digest"] = capabilityDigest
	if !write(state) {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.Contains(line, `"query_state"`) {
			if !write(state) {
				return
			}
			continue
		}
		command, err := wire.Decode([]byte(line))
		if err != nil {
			return
		}
		if !write(map[string]any{
			"message_type": "receipt", "protocol_version": float64(1),
			"command_id": command["command_id"], "boot_id": "boot-A",
			"accepted": true, "received_mono_us": float64(1),
		}) {
			return
		}
		status := "executed"
		if command["operation"] == "safe_stop" {
			status = "safe_state"
		}
		if !write(map[string]any{
			"message_type": "result", "protocol_version": float64(1),
			"command_id": command["command_id"], "boot_id": "boot-A",
			"status": status, "completed_mono_us": float64(1),
		}) {
			return
		}
	}
}

func TestGatewayEffectorDrivesDeviceOverUDS(t *testing.T) {
	catalog := loadThermalCatalog(t)
	catalogDigest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := os.MkdirTemp("/tmp", "as-emul")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	socket := filepath.Join(dir, "d.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		devicePeer(t, conn, catalogDigest)
	}()

	// The SQLite-backed control plane (DB open, migrations, lease claim) is
	// setup, not the operation under test: it must not run inside the dial's
	// timing budget, because under a loaded -race CI runner it alone can
	// outlast a tight window and leave the effector dialing on an expired
	// context (surfacing as a bogus "dial unix: i/o timeout").
	control := newDeviceControl(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transport, err := device.DialUDSTransport(ctx, socket)
	if err != nil {
		t.Fatalf("dial device gateway: %v", err)
	}
	effector, closeFn, err := device.NewGatewayEffector(ctx, device.GatewayEffectorConfig{
		Transport:              transport,
		Catalog:                catalog,
		AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)},
		OwnerEpoch:             "epoch-1",
		OwnerInstance:          "instance-1",
		Authority:              control.authority,
	})
	if err != nil {
		t.Fatalf("open gateway effector: %v", err)
	}
	defer func() { _ = closeFn() }()

	// A governed semantic command is materialized to a bounded device command,
	// sent over the UDS, and accepted — verification pending until an independent
	// reading confirms it (receipt is not proof of effect).
	effect, err := effector.Dispatch(ctx, actionport.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err != nil {
		t.Fatalf("dispatch over UDS: %v", err)
	}
	if !effect.VerificationPending {
		t.Fatal("an accepted device command must leave verification pending, not proven")
	}
	effector = effector.WithTelemetry(nil)
	authorized, err := effector.DispatchAuthorized(ctx, actionport.Command{
		CommandID: "cmd-2", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: "sha256:" + strings.Repeat("e", 64), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}, actionport.Authorization{Check: func(context.Context) error { return nil }})
	if err != nil || !authorized.VerificationPending {
		t.Fatalf("authorized dispatch = %+v, %v", authorized, err)
	}
	status, evidence, err := effector.VerifyDeviceCommand(ctx, actionport.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err != nil || (status != "succeeded" && status != "failed") || evidence["evidence_digest"] == nil {
		t.Fatalf("verification = %q, %v, %v", status, evidence, err)
	}
	if _, err := effector.SafeStop(ctx, "fan-01"); err != nil {
		t.Fatalf("safe stop over UDS: %v", err)
	}
}

func TestFallbackEffectors(t *testing.T) {
	t.Parallel()
	command := actionport.Command{CommandID: "c", EffectorRoute: "notify", IdempotencyKey: idemKey()}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	if effect, err := device.NewSimulatedEffector().DispatchAuthorized(t.Context(), command, allow); err != nil || effect.ProviderResult["accepted"] != true {
		t.Fatalf("simulated effector = %+v, %v", effect, err)
	}
	if _, err := device.NewFailClosedEffector(device.EffectProfilePhysical).DispatchAuthorized(t.Context(), command, allow); err == nil {
		t.Fatal("fail-closed effector accepted an unmapped route")
	}
}
