package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// assertExperimentLedger checks the rows RUNBOOK-G1 confirms by hand: one
// Situation, one decision, one approved R1 intent, one command with a verified
// outcome, and the malformed line quarantined rather than dropped.
func assertExperimentLedger(t *testing.T, dbPath string) {
	t.Helper()
	db := openReadOnly(t, dbPath)
	checks := []struct {
		name, query string
		want        int
	}{
		{"situations", "SELECT COUNT(*) FROM situations", 1},
		{"decisions", "SELECT COUNT(*) FROM decisions", 1},
		{"approved set_indicator intents", "SELECT COUNT(*) FROM intents WHERE intent_type = 'set_indicator' AND policy_status = 'approved'", 1},
		{"commands", "SELECT COUNT(*) FROM commands", 1},
		{"verifications", "SELECT COUNT(*) FROM verifications", 1},
		{"quarantined lines", "SELECT COUNT(*) FROM event_quarantine", 1},
	}
	for _, check := range checks {
		var got int
		if err := db.QueryRowContext(t.Context(), check.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if got != check.want {
			t.Errorf("%s = %d, want %d; ledger: %s", check.name, got, check.want, ledgerSummary(t, db))
		}
	}
}

// assertDeviceReceivedPolicyDigest checks the one device command carries the
// policy digest the bench gateway allow-lists: the digest of the policy
// document bound to the compiled spec digest.
func assertDeviceReceivedPolicyDigest(t *testing.T, run experimentRun) {
	t.Helper()
	compiled, err := spec.CompileFile(context.Background(), run.specPath)
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		t.Fatal(err)
	}
	var ordinary []map[string]any
	for _, command := range run.device.received() {
		if command["operation"] != "safe_stop" {
			ordinary = append(ordinary, command)
		}
	}
	if len(ordinary) != 1 {
		t.Fatalf("device received %d ordinary commands, want 1: %v", len(ordinary), ordinary)
	}
	command := ordinary[0]
	if command["target"] != "led-01" || command["operation"] != "set_led" || command["policy_digest"] != policyDigest {
		t.Errorf("device command = %v, want set_led on led-01 with policy digest %s", command, policyDigest)
	}
}

// assertRunArtifactVerifies exports the run and verifies it, as the runbook's
// last step does.
func assertRunArtifactVerifies(t *testing.T, run experimentRun) {
	t.Helper()
	output := filepath.Join(run.dir, "artifact")
	export := newExportRunCommand()
	export.SetArgs([]string{"--db", run.db, "--tenant", "default", "--output", output})
	if err := export.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("export-run: %v", err)
	}
	verify := newVerifyRunCommand()
	verify.SetArgs([]string{output})
	if err := verify.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("verify-run: %v", err)
	}
	var verdict struct {
		Verdict string `json:"verdict"`
	}
	data, err := os.ReadFile(filepath.Join(output, "verdict.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &verdict); err != nil || verdict.Verdict != "pass" {
		t.Fatalf("run verdict = %s (%v)", data, err)
	}
}
