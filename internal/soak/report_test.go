package soak_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/soak"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestComputePassesCompletePhysicalTransitions(t *testing.T) {
	db, _ := openSoakDB(t)
	ledger := &storage.SafetyLedger{DB: db}
	if err := ledger.Record(t.Context(), storage.SafetyEvent{Type: "physical_transition", Target: "fan-01", Details: completeEvidence()}); err != nil {
		t.Fatal(err)
	}
	report, err := soak.Compute(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "pass" || report.EvidenceCompleteness.Ratio != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestComputeFailsUnresolvedActionOutcome(t *testing.T) {
	db, _ := openSoakDB(t)
	zeroDigest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands
		(command_id, intent_id, tenant_id, effector_route, normalized_target,
		 idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES ('cmd-unknown', 'intent-missing', 'tenant', 'safe_stop', 'fan-01', ?, ?, ?, 'reconciling', '2026-08-29T12:00:00Z', '2026-08-29T12:00:00Z')`, zeroDigest, []byte("{}"), zeroDigest); err != nil {
		t.Fatal(err)
	}
	report, err := soak.Compute(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.Diagnostics["unresolved_action_outcomes"] != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestComputeFailsAnyZeroToleranceEventOrIncompleteEvidence(t *testing.T) {
	db, _ := openSoakDB(t)
	ledger := &storage.SafetyLedger{DB: db}
	for _, event := range []storage.SafetyEvent{
		{Type: "unsafe_output", Target: "fan-01"},
		{Type: "physical_transition", Target: "fan-01", Details: map[string]any{"evidence_complete": false}},
	} {
		if err := ledger.Record(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	report, err := soak.Compute(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.ZeroTolerance.UnsafeOutputCount != 1 || report.EvidenceCompleteness.Ratio != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestComputeFailsWithActiveReconciliationBarrier(t *testing.T) {
	db, _ := openSoakDB(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO device_reconciliation
		(device_id, boot_id, status, opening_boot_id, state_json, state_sha256, authority_epoch, opened_at, updated_at)
		VALUES ('thermal-01', 'boot-B', 'required', 'boot-B', ?, ?, 'epoch-1', '2026-08-29T12:00:00.000000000Z', '2026-08-29T12:00:00.000000000Z')`, []byte("{}"), make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	report, err := soak.Compute(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.Diagnostics["reconciliation_barriers"] != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func completeEvidence() map[string]any {
	return map[string]any{
		"evidence_complete": true,
		"source":            "independent-feedback",
		"evidence_digest":   "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}
}

func openSoakDB(t *testing.T) (*storage.DB, string) {
	t.Helper()
	db, err := storage.Open(t.Context(), t.TempDir()+"/soak.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ""
}
