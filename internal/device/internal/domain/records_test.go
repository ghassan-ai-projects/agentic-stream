package domain

import (
	"strings"
	"testing"
)

func TestResultMatchesCommandUsesExplicitReceiptResultMatrix(t *testing.T) {
	expired, notReady := "expired", "not_ready"
	command := Command{CommandID: "cmd-1", Operation: "set_led"}
	cases := []struct {
		name    string
		receipt Receipt
		result  Result
		matches bool
	}{
		{name: "accepted ordinary executed", receipt: Receipt{Accepted: true}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "executed"}, matches: true},
		{name: "accepted ordinary safe state", receipt: Receipt{Accepted: true}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "safe_state"}},
		{name: "accepted ordinary expired", receipt: Receipt{Accepted: true}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "expired"}},
		{name: "rejected matching code", receipt: Receipt{Accepted: false, RejectCode: &expired}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "rejected", ErrorCode: &expired}, matches: true},
		{name: "rejected wrong status", receipt: Receipt{Accepted: false, RejectCode: &expired}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "expired", ErrorCode: &expired}},
		{name: "rejected wrong code", receipt: Receipt{Accepted: false, RejectCode: &expired}, result: Result{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A", Status: "rejected", ErrorCode: &notReady}},
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
	command := Command{CommandID: "safe-stop/fan-01", Operation: "safe_stop"}
	receipt := Receipt{Accepted: true}
	for _, status := range []string{"executed", "rejected", "expired", "superseded"} {
		result := Result{MessageType: "result", CommandID: command.CommandID, BootID: "boot-A", Status: status}
		if ResultMatches(result, command, "boot-A", receipt) {
			t.Fatalf("safe-stop status %q was accepted", status)
		}
	}
	result := Result{MessageType: "result", CommandID: command.CommandID, BootID: "boot-A", Status: "safe_state"}
	if !ResultMatches(result, command, "boot-A", receipt) {
		t.Fatal("safe_state result was rejected")
	}
}

func TestReceiptMatchesCommandAndBoot(t *testing.T) {
	t.Parallel()
	command := Command{CommandID: "cmd-1"}
	receipt := Receipt{MessageType: "receipt", CommandID: "cmd-1", BootID: "boot-A"}
	if !ReceiptMatches(receipt, command, "boot-A") || ReceiptMatches(receipt, command, "boot-B") {
		t.Fatal("receipt boot binding")
	}
	if ReceiptMatches(Receipt{MessageType: "result", CommandID: "cmd-1", BootID: "boot-A"}, command, "boot-A") {
		t.Fatal("a result was accepted as a receipt")
	}
}

func TestCheckStateRequiresAllowedIdentity(t *testing.T) {
	t.Parallel()
	catalogDigest, firmware := "sha256:"+strings.Repeat("d", 64), "sha256:"+strings.Repeat("c", 64)
	allowed := StateAllowlist{ProtocolVersion: 1, CatalogDigest: catalogDigest, CapabilityDigest: []string{catalogDigest}, FirmwareDigests: []string{firmware}}
	valid := State{MessageType: "state", ProtocolVersion: 1, DeviceID: "d", BootID: "b", CapabilityDigest: catalogDigest, FirmwareDigest: firmware}
	for _, tt := range []struct {
		name   string
		mutate func(*State)
		want   string
	}{
		{"valid", func(*State) {}, ""},
		{"not a state", func(s *State) { s.MessageType = "receipt" }, "want state"},
		{"protocol", func(s *State) { s.ProtocolVersion = 2 }, "protocol version 2"},
		{"capability", func(s *State) { s.CapabilityDigest = firmware }, "capability digest"},
		{"firmware", func(s *State) { s.FirmwareDigest = catalogDigest }, "firmware digest"},
		{"identity", func(s *State) { s.BootID = "" }, "identity is incomplete"},
	} {
		state := valid
		tt.mutate(&state)
		err := CheckState(state, allowed)
		if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: CheckState = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestCommandIdentityIgnoresCommandID(t *testing.T) {
	t.Parallel()
	first, err := Command{CommandID: "a", Target: "fan-01"}.Identity()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Command{CommandID: "b", Target: "fan-01"}.Identity()
	other, _ := Command{CommandID: "a", Target: "led-01"}.Identity()
	if first != second || first == other {
		t.Fatal("command identity must ignore only the command ID")
	}
	if digest, err := StateDigest(map[string]any{"a": 1}); err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("state digest = %q, %v", digest, err)
	}
}
