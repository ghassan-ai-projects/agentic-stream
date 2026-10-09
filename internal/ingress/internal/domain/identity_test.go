package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestQuarantineIdentitiesAreScopedToTheirSource(t *testing.T) {
	t.Parallel()
	if got := QuarantineID("replay:a", 3); got != "replay:a:line:3" {
		t.Fatalf("QuarantineID = %q", got)
	}
	if QuarantineID("replay:b", 3) == QuarantineID("replay:a", 3) {
		t.Fatal("two connectors share a quarantine identity for the same line")
	}
	if got := LiveQuarantineID("inst", 2, 9); got != "live-uds:inst:2:9" {
		t.Fatalf("LiveQuarantineID = %q", got)
	}
	if LiveQuarantineID("inst", 2, 9) == LiveQuarantineID("inst", 9, 2) {
		t.Fatal("connection and line number are interchangeable")
	}
}

func TestConnectorIdentitiesDefaultFromThePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"jsonl default", JSONLConnectorID("", "/t.jsonl"), "jsonl:/t.jsonl"},
		{"jsonl given", JSONLConnectorID("x", "/t.jsonl"), "x"},
		{"simulator default", SimulatorConnectorID("", "/t.jsonl"), "simulator-jsonl:/t.jsonl"},
		{"simulator given", SimulatorConnectorID("y", "/t.jsonl"), "y"},
		{"tenant default", TenantOrDefault(""), contractsv1.TenantID},
		{"tenant given", TenantOrDefault("acme"), "acme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
