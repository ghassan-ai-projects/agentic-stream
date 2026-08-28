package contractsv1_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Device wire schemas for the serial effector boundary (Real-World Sensor
// HIL-0, Phase 03 Task 3.1). The contract is the single source of truth shared
// with the gateway/emulator/firmware repos. These tests pin the golden frames
// and prove the codec fails closed on malformed, oversized, wrong-version,
// unknown-type, and unknown-field frames — an invalid frame must never validate
// into a usable record.

func goldenDeviceCommand() map[string]any {
	return map[string]any{
		"message_type":       "command",
		"protocol_version":   float64(1),
		"command_id":         "cmd-01",
		"idempotency_key":    "sha256:" + repeat64('a'),
		"target":             "fan-01",
		"operation":          "set_pwm_lease",
		"parameters":         map[string]any{"duty_permille": float64(450), "lease_ms": float64(5000)},
		"expected_boot_id":   "boot-A",
		"not_before_mono_us": float64(5200000),
		"expires_after_ms":   float64(2000),
		"policy_digest":      "sha256:" + repeat64('b'),
	}
}

func goldenDeviceReceipt() map[string]any {
	return map[string]any{
		"message_type":     "receipt",
		"protocol_version": float64(1),
		"command_id":       "cmd-01",
		"boot_id":          "boot-A",
		"accepted":         true,
		"received_mono_us": float64(5200500),
	}
}

func goldenDeviceResult() map[string]any {
	return map[string]any{
		"message_type":     "result",
		"protocol_version": float64(1),
		"command_id":       "cmd-01",
		"boot_id":          "boot-A",
		"status":           "executed",
		"detail":           "lease active",
	}
}

func goldenDeviceState() map[string]any {
	return map[string]any{
		"message_type":      "state",
		"protocol_version":  float64(1),
		"device_id":         "dev-01",
		"boot_id":           "boot-A",
		"firmware_digest":   "sha256:" + repeat64('c'),
		"capability_digest": "sha256:" + repeat64('d'),
		"safe_state":        true,
		"current_output":    map[string]any{"target": "fan-01", "operation": "set_pwm_lease", "value": float64(0), "energized": false},
		"dedup_ledger":      map[string]any{"persistent": false, "size": float64(0)},
	}
}

func TestDeviceWireGoldenFramesValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		schema contractsv1.SchemaName
		doc    map[string]any
	}{
		{"command", contractsv1.SchemaDeviceCommand, goldenDeviceCommand()},
		{"receipt", contractsv1.SchemaDeviceReceipt, goldenDeviceReceipt()},
		{"result", contractsv1.SchemaDeviceResult, goldenDeviceResult()},
		{"state", contractsv1.SchemaDeviceState, goldenDeviceState()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := contractsv1.Validate(tc.schema, tc.doc); err != nil {
				t.Fatalf("golden %s frame must validate: %v", tc.name, err)
			}
		})
	}
}

func TestDeviceWireFramesFailClosed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		schema contractsv1.SchemaName
		mutate func(map[string]any)
	}{
		{"command wrong message_type", contractsv1.SchemaDeviceCommand, func(d map[string]any) { d["message_type"] = "receipt" }},
		{"command unknown field", contractsv1.SchemaDeviceCommand, func(d map[string]any) { d["pin"] = float64(13) }},
		{"command missing operation", contractsv1.SchemaDeviceCommand, func(d map[string]any) { delete(d, "operation") }},
		{"command bad idempotency_key", contractsv1.SchemaDeviceCommand, func(d map[string]any) { d["idempotency_key"] = "nope" }},
		{"command protocol_version out of range", contractsv1.SchemaDeviceCommand, func(d map[string]any) { d["protocol_version"] = float64(999) }},
		{"command expires_after_ms zero", contractsv1.SchemaDeviceCommand, func(d map[string]any) { d["expires_after_ms"] = float64(0) }},
		{"receipt unknown reject_code", contractsv1.SchemaDeviceReceipt, func(d map[string]any) { d["accepted"] = false; d["reject_code"] = "gremlin" }},
		{"receipt missing boot_id", contractsv1.SchemaDeviceReceipt, func(d map[string]any) { delete(d, "boot_id") }},
		{"result bad status", contractsv1.SchemaDeviceResult, func(d map[string]any) { d["status"] = "maybe" }},
		{"state bad firmware_digest", contractsv1.SchemaDeviceState, func(d map[string]any) { d["firmware_digest"] = "sha256:short" }},
		{"state missing capability_digest", contractsv1.SchemaDeviceState, func(d map[string]any) { delete(d, "capability_digest") }},
	}
	golden := map[contractsv1.SchemaName]func() map[string]any{
		contractsv1.SchemaDeviceCommand: goldenDeviceCommand,
		contractsv1.SchemaDeviceReceipt: goldenDeviceReceipt,
		contractsv1.SchemaDeviceResult:  goldenDeviceResult,
		contractsv1.SchemaDeviceState:   goldenDeviceState,
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc := golden[tc.schema]()
			tc.mutate(doc)
			if err := contractsv1.Validate(tc.schema, doc); err == nil {
				t.Fatalf("mutated %s frame must fail closed, but validated", tc.name)
			}
		})
	}
}

func repeat64(b byte) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = b
	}
	return string(out)
}
