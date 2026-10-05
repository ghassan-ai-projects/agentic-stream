package domain

import (
	"strings"
	"testing"
)

func TestResultMatchesCommandUsesExplicitReceiptResultMatrix(t *testing.T) {
	command := map[string]any{"command_id": "cmd-1", "operation": "set_led"}
	cases := []struct {
		name    string
		receipt map[string]any
		result  map[string]any
		matches bool
	}{
		{name: "accepted ordinary executed", receipt: map[string]any{"accepted": true}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "executed"}, matches: true},
		{name: "accepted ordinary safe state", receipt: map[string]any{"accepted": true}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "safe_state"}},
		{name: "accepted ordinary expired", receipt: map[string]any{"accepted": true}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "expired"}},
		{name: "rejected matching code", receipt: map[string]any{"accepted": false, "reject_code": "expired"}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "rejected", "error_code": "expired"}, matches: true},
		{name: "rejected wrong status", receipt: map[string]any{"accepted": false, "reject_code": "expired"}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "expired", "error_code": "expired"}},
		{name: "rejected wrong code", receipt: map[string]any{"accepted": false, "reject_code": "expired"}, result: map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A", "status": "rejected", "error_code": "not_ready"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResultMatches(tc.result, command, "boot-A", tc.receipt); got != tc.matches {
				t.Fatalf("result match=%v, want %v", got, tc.matches)
			}
		})
	}
}

func TestResultMatchesCommandRequiresSafeStateForSafeStop(t *testing.T) {
	command := map[string]any{"command_id": "safe-stop/fan-01", "operation": "safe_stop"}
	receipt := map[string]any{"accepted": true}
	for _, status := range []string{"executed", "rejected", "expired", "superseded"} {
		result := map[string]any{"message_type": "result", "command_id": command["command_id"], "boot_id": "boot-A", "status": status}
		if ResultMatches(result, command, "boot-A", receipt) {
			t.Fatalf("safe-stop status %q was accepted", status)
		}
	}
	result := map[string]any{"message_type": "result", "command_id": command["command_id"], "boot_id": "boot-A", "status": "safe_state"}
	if !ResultMatches(result, command, "boot-A", receipt) {
		t.Fatal("safe_state result was rejected")
	}
}

func TestReceiptMatchesCommandAndBoot(t *testing.T) {
	t.Parallel()
	command := map[string]any{"command_id": "cmd-1"}
	receipt := map[string]any{"message_type": "receipt", "command_id": "cmd-1", "boot_id": "boot-A"}
	if !ReceiptMatches(receipt, command, "boot-A") || ReceiptMatches(receipt, command, "boot-B") {
		t.Fatal("receipt boot binding")
	}
	if ReceiptMatches(map[string]any{"message_type": "result", "command_id": "cmd-1", "boot_id": "boot-A"}, command, "boot-A") {
		t.Fatal("a result was accepted as a receipt")
	}
}

func TestCheckStateRequiresAllowedIdentity(t *testing.T) {
	t.Parallel()
	catalogDigest, firmware := "sha256:"+strings.Repeat("d", 64), "sha256:"+strings.Repeat("c", 64)
	allowed := StateAllowlist{ProtocolVersion: 1, CatalogDigest: catalogDigest, CapabilityDigest: []string{catalogDigest}, FirmwareDigests: []string{firmware}}
	valid := func() map[string]any {
		return map[string]any{"message_type": "state", "protocol_version": float64(1), "device_id": "d", "boot_id": "b",
			"capability_digest": catalogDigest, "firmware_digest": firmware}
	}
	for _, tt := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"valid", func(map[string]any) {}, ""},
		{"not a state", func(s map[string]any) { s["message_type"] = "receipt" }, "want state"},
		{"protocol", func(s map[string]any) { s["protocol_version"] = float64(2) }, "protocol version 2"},
		{"capability", func(s map[string]any) { s["capability_digest"] = firmware }, "capability digest"},
		{"firmware", func(s map[string]any) { s["firmware_digest"] = catalogDigest }, "firmware digest"},
		{"identity", func(s map[string]any) { s["boot_id"] = "" }, "identity is incomplete"},
	} {
		state := valid()
		tt.mutate(state)
		err := CheckState(state, allowed)
		if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: CheckState = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestCommandIdentityIgnoresCommandID(t *testing.T) {
	t.Parallel()
	first, err := CommandIdentity(map[string]any{"command_id": "a", "target": "fan-01"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := CommandIdentity(map[string]any{"command_id": "b", "target": "fan-01"})
	other, _ := CommandIdentity(map[string]any{"command_id": "a", "target": "led-01"})
	if first != second || first == other {
		t.Fatal("command identity must ignore only the command ID")
	}
	if digest, err := StateDigest(map[string]any{"a": 1}); err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("state digest = %q, %v", digest, err)
	}
}
