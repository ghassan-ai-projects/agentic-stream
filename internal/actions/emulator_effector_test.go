package actions_test

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// devicePeer is an in-process device that speaks the wire contract over a UDS
// with the digests this session expects, so NewEmulatorEffector can complete a
// real handshake + governed command exchange without a cross-repo binary. It is
// the same behaviour `streamsim device serve` provides.
func devicePeer(t *testing.T, conn net.Conn, capabilityDigest string) {
	t.Helper()
	defer conn.Close()
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
		command, err := actions.DecodeDeviceRecord([]byte(line))
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
	}
}

func TestEmulatorEffectorDrivesDeviceOverUDS(t *testing.T) {
	catalog := loadThermalCatalog(t)
	catalogDigest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := os.MkdirTemp("/tmp", "as-emul")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "d.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		devicePeer(t, conn, catalogDigest)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	control := newDeviceControl(t)

	effector, closeFn, err := actions.NewEmulatorEffector(ctx, actions.EmulatorEffectorConfig{
		SocketPath:             socket,
		Catalog:                catalog,
		AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)},
		AuthorityEpoch:         "epoch-1",
		OwnerInstance:          "instance-1",
		Authority:              control.authority,
		Reconciliation:         control.reconciliation,
	})
	if err != nil {
		t.Fatalf("open emulator effector: %v", err)
	}
	defer closeFn()

	// A governed semantic command is materialized to a bounded device command,
	// sent over the UDS, and accepted — verification pending until an independent
	// reading confirms it (receipt is not proof of effect).
	effect, err := effector.Dispatch(ctx, actions.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err != nil {
		t.Fatalf("dispatch over UDS: %v", err)
	}
	if !effect.VerificationPending {
		t.Fatal("an accepted device command must leave verification pending, not proven")
	}
}
