package device_test

import (
	"bufio"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

type emulatedDevice struct {
	capabilityDigest string
	output           map[string]any
}

func startEmulatedDevice(t *testing.T, capabilityDigest string) string {
	t.Helper()
	socket := filepath.Join(workerfake.SocketDir(t), "d.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("listen on %s: %v", socket, err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		(&emulatedDevice{capabilityDigest: capabilityDigest}).serve(conn)
	}()
	return socket
}

func (d *emulatedDevice) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if !d.write(conn, d.state()) {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if !d.answer(conn, line) {
			return
		}
	}
}

func (d *emulatedDevice) answer(conn net.Conn, line string) bool {
	if strings.Contains(line, `"query_state"`) {
		return d.write(conn, d.state())
	}
	command, err := wire.Decode([]byte(line))
	if err != nil {
		return false
	}
	d.apply(command)
	return d.write(conn, map[string]any{
		"message_type": "receipt", "protocol_version": float64(1),
		"command_id": command["command_id"], "boot_id": "boot-A",
		"accepted": true, "received_mono_us": float64(1),
	}) && d.write(conn, map[string]any{
		"message_type": "result", "protocol_version": float64(1),
		"command_id": command["command_id"], "boot_id": "boot-A",
		"status": d.resultStatus(command), "completed_mono_us": float64(1),
	})
}

func (d *emulatedDevice) resultStatus(command map[string]any) string {
	if command["operation"] == "safe_stop" {
		return "safe_state"
	}
	return "executed"
}

func (d *emulatedDevice) apply(command map[string]any) {
	parameters, _ := command["parameters"].(map[string]any)
	switch command["operation"] {
	case "set_led":
		d.output = outputOf(command, parameters["brightness_permille"])
	case "set_pwm_lease":
		d.output = outputOf(command, parameters["duty_permille"])
	case "safe_stop":
		d.output = nil
	}
}

func outputOf(command map[string]any, value any) map[string]any {
	level, _ := value.(float64)
	return map[string]any{"target": command["target"], "operation": command["operation"], "value": level, "energized": level > 0}
}

func (d *emulatedDevice) state() map[string]any {
	state := map[string]any{
		"message_type": "state", "protocol_version": float64(1), "device_id": "thermal-01", "boot_id": "boot-A",
		"firmware_digest": firmwareDigest(), "capability_digest": d.capabilityDigest, "safe_state": d.output == nil,
	}
	if d.output != nil {
		state["current_output"] = d.output
	}
	return state
}

func (d *emulatedDevice) write(conn net.Conn, document map[string]any) bool {
	frame, err := canonicaljson.Marshal(document)
	if err != nil {
		return false
	}
	_, err = conn.Write(append(frame, '\n'))
	return err == nil
}
