package actions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func goldenDeviceState() map[string]any {
	return map[string]any{
		"message_type":      "state",
		"protocol_version":  1,
		"device_id":         "thermal-01",
		"boot_id":           "boot-A",
		"firmware_digest":   "sha256:" + strings.Repeat("c", 64),
		"capability_digest": "sha256:" + strings.Repeat("d", 64),
		"safe_state":        true,
	}
}

func goldenDeviceReceipt() map[string]any {
	return map[string]any{
		"message_type":     "receipt",
		"protocol_version": 1,
		"command_id":       "cmd-1",
		"boot_id":          "boot-A",
		"accepted":         true,
	}
}

func goldenDeviceResult() map[string]any {
	return map[string]any{
		"message_type":     "result",
		"protocol_version": 1,
		"command_id":       "cmd-1",
		"boot_id":          "boot-A",
		"status":           "executed",
	}
}

func goldenDeviceCommand() map[string]any {
	return map[string]any{
		"message_type":       "command",
		"protocol_version":   1,
		"command_id":         "cmd-1",
		"idempotency_key":    idemKey(),
		"target":             "led-01",
		"operation":          "set_led",
		"parameters":         map[string]any{"brightness_permille": 500, "pattern": "slow_blink"},
		"expected_boot_id":   "boot-A",
		"not_before_mono_us": 0,
		"expires_after_ms":   1000,
		"policy_digest":      policyKey(),
	}
}

func TestDeviceCodecAcceptsCanonicalRecords(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]map[string]any{
		"command": goldenDeviceCommand(),
		"receipt": goldenDeviceReceipt(),
		"result":  goldenDeviceResult(),
		"state":   goldenDeviceState(),
	} {
		name, document := name, document
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			frame, err := actions.EncodeDeviceRecord(document)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !bytes.HasSuffix(frame, []byte{'\n'}) || bytes.Count(frame, []byte{'\n'}) != 1 {
				t.Fatalf("frame is not one NDJSON line: %q", frame)
			}
			decoded, err := actions.DecodeDeviceRecord(frame)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if err := contractsv1.Validate(schemaForMessageType(decoded), decoded); err != nil {
				t.Fatalf("decoded record is not schema-valid: %v", err)
			}
		})
	}
}

func TestDeviceCodecFailsClosed(t *testing.T) {
	t.Parallel()
	valid, err := actions.EncodeDeviceRecord(goldenDeviceCommand())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"empty":           nil,
		"malformed":       []byte("{\"message_type\":"),
		"trailing record": append(append([]byte(nil), valid...), []byte("{\"message_type\":\"receipt\"}")...),
		"wrong version":   bytes.Replace(valid, []byte("\"protocol_version\":1"), []byte("\"protocol_version\":2"), 1),
		"unknown type":    bytes.Replace(valid, []byte("\"message_type\":\"command\""), []byte("\"message_type\":\"motor\""), 1),
		"unknown field":   bytes.Replace(valid, []byte("\"target\":\"led-01\""), []byte("\"pin\":13,\"target\":\"led-01\""), 1),
	}
	for name, frame := range cases {
		name, frame := name, frame
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := actions.DecodeDeviceRecord(frame); err == nil {
				t.Fatalf("invalid %s frame was accepted", name)
			}
		})
	}
	oversized := append(append([]byte(nil), valid...), bytes.Repeat([]byte{' '}, 64*1024)...)
	if _, err := actions.DecodeDeviceRecord(oversized); err == nil {
		t.Fatal("oversized frame was accepted")
	}
	if _, err := actions.EncodeDeviceRecord(map[string]any{
		"message_type": "command", "protocol_version": 1, "command_id": "cmd-1", "idempotency_key": idemKey(),
		"target": "led-01", "operation": "set_led", "parameters": map[string]any{"padding": strings.Repeat("x", 70*1024)},
		"expected_boot_id": "boot-A", "not_before_mono_us": 0, "expires_after_ms": 1000, "policy_digest": policyKey(),
	}); err == nil {
		t.Fatal("oversized encoded frame was accepted")
	}
}

func FuzzDecodeDeviceRecord(f *testing.F) {
	valid, err := actions.EncodeDeviceRecord(goldenDeviceCommand())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte("{\"message_type\":\"command\",\"protocol_version\":999}"))
	f.Fuzz(func(t *testing.T, frame []byte) {
		document, err := actions.DecodeDeviceRecord(frame)
		if err != nil {
			return
		}
		if document["message_type"] == "command" {
			if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
				t.Fatalf("codec returned invalid command: %v", err)
			}
		}
	})
}

func schemaForMessageType(document map[string]any) contractsv1.SchemaName {
	switch document["message_type"] {
	case "command":
		return contractsv1.SchemaDeviceCommand
	case "receipt":
		return contractsv1.SchemaDeviceReceipt
	case "result":
		return contractsv1.SchemaDeviceResult
	default:
		return contractsv1.SchemaDeviceState
	}
}
