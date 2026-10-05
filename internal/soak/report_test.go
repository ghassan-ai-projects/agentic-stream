package soak_test

import (
	"testing"
	"time"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/soak"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// newAuthority admits epoch-1 of instance-1 and returns its device authority.
func newAuthority(t *testing.T, db *storage.DB) *deviceauthority.Service {
	t.Helper()
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &control.EpochControl{DB: db}})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func TestComputePassesCompletePhysicalTransitions(t *testing.T) {
	db, _ := openSoakDB(t)
	authority := newAuthority(t, db)
	if err := authority.RecordSafetyEvent(t.Context(), deviceauthority.SafetyEvent{Type: "physical_transition", Target: "fan-01", Details: completeEvidence()}); err != nil {
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
	authority := newAuthority(t, db)
	for _, event := range []deviceauthority.SafetyEvent{
		{Type: "unsafe_output", Target: "fan-01"},
		{Type: "physical_transition", Target: "fan-01", Details: map[string]any{"evidence_complete": false}},
	} {
		if err := authority.RecordSafetyEvent(t.Context(), event); err != nil {
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
	authority := newAuthority(t, db)
	owner := deviceauthority.Owner{Epoch: "epoch-1", Instance: "instance-1"}
	for _, boot := range []string{"boot-A", "boot-B"} { // the reboot requires reconciliation
		if _, err := authority.RecordDeviceState(t.Context(), owner, map[string]any{"device_id": "thermal-01", "boot_id": boot}); err != nil {
			t.Fatal(err)
		}
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
