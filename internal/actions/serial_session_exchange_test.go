package actions

import "testing"

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
			if got := resultMatchesCommand(tc.result, command, "boot-A", tc.receipt); got != tc.matches {
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
		if resultMatchesCommand(result, command, "boot-A", receipt) {
			t.Fatalf("safe-stop status %q was accepted", status)
		}
	}
	result := map[string]any{"message_type": "result", "command_id": command["command_id"], "boot_id": "boot-A", "status": "safe_state"}
	if !resultMatchesCommand(result, command, "boot-A", receipt) {
		t.Fatal("safe_state result was rejected")
	}
}
