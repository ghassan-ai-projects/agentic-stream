package contractsv1

import "strings"

// Device wire conformance frames. These are the single, canonical set of
// example device records for the serial-effector boundary (Real-World Sensor
// HIL-0). They are the source of truth the golden JSON fixtures under
// conformance/v1/ are generated from, and the same values the other program
// repos (Streams Simulator device emulator, edge gateway, firmware) test their
// wire compatibility against. See conformance/README.md.
//
// Field convention is snake_case (device wire), never the camelCase of a
// SituationSpec. Digests are fixed 64-hex placeholders so the fixtures are
// deterministic and byte-stable across regenerations.

func digest(b byte) string { return "sha256:" + strings.Repeat(string(b), 64) }

// ConformanceValidFrame returns a fresh copy of the canonical valid frame for
// one device message type ("command", "receipt", "result", "state"). It panics
// on an unknown type; callers pass a fixed literal.
func ConformanceValidFrame(messageType string) map[string]any {
	switch messageType {
	case "command":
		return map[string]any{
			"message_type":       "command",
			"protocol_version":   float64(DeviceProtocolVersion),
			"command_id":         "cmd-01",
			"idempotency_key":    digest('a'),
			"target":             "fan-01",
			"operation":          "set_pwm_lease",
			"parameters":         map[string]any{"duty_permille": float64(450), "lease_ms": float64(5000)},
			"expected_boot_id":   "boot-A",
			"not_before_mono_us": float64(5200000),
			"expires_after_ms":   float64(2000),
			"policy_digest":      digest('b'),
		}
	case "receipt":
		return map[string]any{
			"message_type":     "receipt",
			"protocol_version": float64(DeviceProtocolVersion),
			"command_id":       "cmd-01",
			"boot_id":          "boot-A",
			"accepted":         true,
			"received_mono_us": float64(5200500),
		}
	case "result":
		return map[string]any{
			"message_type":      "result",
			"protocol_version":  float64(DeviceProtocolVersion),
			"command_id":        "cmd-01",
			"boot_id":           "boot-A",
			"status":            "executed",
			"detail":            "lease active",
			"completed_mono_us": float64(5205000),
		}
	case "state":
		return map[string]any{
			"message_type":      "state",
			"protocol_version":  float64(DeviceProtocolVersion),
			"device_id":         "dev-01",
			"boot_id":           "boot-A",
			"firmware_digest":   digest('c'),
			"capability_digest": digest('d'),
			"safe_state":        true,
			"current_output":    map[string]any{"target": "fan-01", "operation": "set_pwm_lease", "value": float64(0), "energized": false},
			"dedup_ledger":      map[string]any{"persistent": false, "size": float64(0)},
		}
	default:
		panic("contractsv1: unknown conformance message_type " + messageType)
	}
}

// ConformanceValidMessageTypes lists the device message types, in wire order.
func ConformanceValidMessageTypes() []string {
	return []string{"command", "receipt", "result", "state"}
}

// InvalidFrame is one negative conformance case: a named frame that MUST be
// rejected by a conforming decoder, with the schema it violates. Consumers use
// these to prove their decoder fails closed, not just that valid frames pass.
type InvalidFrame struct {
	Name   string
	Schema SchemaName
	Doc    map[string]any
}

// ConformanceInvalidFrames returns the negative conformance corpus. Each frame
// is a valid frame with exactly one rule broken, so the reason for rejection is
// unambiguous. A conforming decoder must reject every one.
func ConformanceInvalidFrames() []InvalidFrame {
	mutate := func(messageType string, f func(map[string]any)) map[string]any {
		doc := ConformanceValidFrame(messageType)
		f(doc)
		return doc
	}
	return []InvalidFrame{
		{"command-wrong-message-type", SchemaDeviceCommand, mutate("command", func(d map[string]any) { d["message_type"] = "receipt" })},
		{"command-unknown-field", SchemaDeviceCommand, mutate("command", func(d map[string]any) { d["pin"] = float64(13) })},
		{"command-missing-operation", SchemaDeviceCommand, mutate("command", func(d map[string]any) { delete(d, "operation") })},
		{"command-bad-idempotency-key", SchemaDeviceCommand, mutate("command", func(d map[string]any) { d["idempotency_key"] = "nope" })},
		{"command-protocol-version-out-of-range", SchemaDeviceCommand, mutate("command", func(d map[string]any) { d["protocol_version"] = float64(999) })},
		{"command-missing-policy-digest", SchemaDeviceCommand, mutate("command", func(d map[string]any) { delete(d, "policy_digest") })},
		{"command-expires-after-ms-zero", SchemaDeviceCommand, mutate("command", func(d map[string]any) { d["expires_after_ms"] = float64(0) })},
		{"receipt-unknown-reject-code", SchemaDeviceReceipt, mutate("receipt", func(d map[string]any) { d["accepted"] = false; d["reject_code"] = "gremlin" })},
		{"receipt-rejected-without-reject-code", SchemaDeviceReceipt, mutate("receipt", func(d map[string]any) { d["accepted"] = false })},
		{"receipt-missing-boot-id", SchemaDeviceReceipt, mutate("receipt", func(d map[string]any) { delete(d, "boot_id") })},
		{"result-bad-status", SchemaDeviceResult, mutate("result", func(d map[string]any) { d["status"] = "maybe" })},
		{"state-bad-firmware-digest", SchemaDeviceState, mutate("state", func(d map[string]any) { d["firmware_digest"] = "sha256:short" })},
		{"state-missing-capability-digest", SchemaDeviceState, mutate("state", func(d map[string]any) { delete(d, "capability_digest") })},
	}
}

// SchemaForMessageType maps a device message_type to its schema name.
func SchemaForMessageType(messageType string) (SchemaName, bool) {
	switch messageType {
	case "command":
		return SchemaDeviceCommand, true
	case "receipt":
		return SchemaDeviceReceipt, true
	case "result":
		return SchemaDeviceResult, true
	case "state":
		return SchemaDeviceState, true
	default:
		return "", false
	}
}
