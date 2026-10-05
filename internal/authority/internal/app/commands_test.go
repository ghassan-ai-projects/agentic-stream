package app_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

func TestBindCommandIsIdempotentAndRefusesConflicts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	binding := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: ownerOne,
		CommandDigest: "sha256:abababababababababababababababababababababababababababababababab"}
	for range 2 {
		if err := f.service.BindCommand(t.Context(), binding); err != nil {
			t.Fatalf("bind command: %v", err)
		}
	}
	conflict := binding
	conflict.Device = bootB
	if err := f.service.BindCommand(t.Context(), conflict); err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("conflicting binding = %v", err)
	}
	if err := f.service.BindCommand(t.Context(), domain.CommandBinding{CommandID: "cmd-2", Owner: ownerOne}); err == nil {
		t.Fatal("incomplete binding was accepted")
	}
	if f.count(t, `SELECT COUNT(*) FROM device_command_bindings`) != 1 {
		t.Fatal("binding was not recorded exactly once")
	}
}

func TestVerifyCommandEvidenceUsesTheBinding(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	state, _ := f.recordState(t, bootA)
	binding := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: ownerOne}
	if err := f.service.BindCommand(t.Context(), binding); err != nil {
		t.Fatal(err)
	}
	evidence := evidenceFor(t, state, "fan-01")
	verify := func(commandID, target string) error {
		return f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return app.VerifyCommandEvidence(t.Context(), store.Join(tx), domain.CommandEvidence{CommandID: commandID, Target: target, Evidence: evidence})
		})
	}
	if err := verify("cmd-1", "fan-01"); err != nil {
		t.Fatalf("valid evidence: %v", err)
	}
	if err := verify("cmd-1", "led-01"); err == nil || !strings.Contains(err.Error(), "binding target does not match") {
		t.Fatalf("other target = %v", err)
	}
	if err := verify("unbound", "led-01"); err != nil {
		t.Fatalf("unbound command needs no device evidence: %v", err)
	}
}
