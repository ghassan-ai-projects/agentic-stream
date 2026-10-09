package wire_test

import (
	"strings"
	"testing"
)

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func policyKey() string { return "sha256:" + strings.Repeat("b", 64) }

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

func assertRefusal(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}
