package app

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// newAuthority admits epoch-1 of instance-1 and returns its device authority.
func newAuthority(t *testing.T, db *storage.DB) *deviceauthority.Service {
	t.Helper()
	owner := &control.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &control.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func TestSoakReportPassesCompletePhysicalTransitions(t *testing.T) {
	t.Parallel()
	db, _ := openSoakDB(t)
	authority := newAuthority(t, db)
	if err := authority.RecordSafetyEvent(t.Context(), deviceauthority.SafetyEvent{Type: "physical_transition", Target: "fan-01", Details: completeEvidence()}); err != nil {
		t.Fatal(err)
	}
	report, err := computeSoak(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "pass" || report.EvidenceCompleteness.Ratio != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestSoakReportFailsUnresolvedActionOutcome(t *testing.T) {
	t.Parallel()
	db, _ := openSoakDB(t)
	insertCommand(t, db, "cmd-unknown", actionport.CommandReconciling)
	report, err := computeSoak(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.Diagnostics["unresolved_action_outcomes"] != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestSoakReportCountsEveryCommandStatusLikeTheReconciliationBarrier(t *testing.T) {
	t.Parallel()
	for _, status := range []string{
		actionport.CommandPending, actionport.CommandDispatching, actionport.CommandSucceeded, actionport.CommandFailed,
		actionport.CommandReconciling, actionport.CommandOutcomeUnknown, actionport.CommandManualReview,
	} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			db, _ := openSoakDB(t)
			insertCommand(t, db, "cmd-1", status)
			report, err := computeSoak(t, db)
			if err != nil {
				t.Fatal(err)
			}
			want, failing := uint64(0), false
			if actionport.IsUnresolvedCommandStatus(status) {
				want, failing = 1, true
			}
			barrier := countBarrier(t, db, "cmd-1")
			if soak := report.Diagnostics["unresolved_action_outcomes"]; soak != want || barrier != int64(want) || (report.Verdict == "fail") != failing {
				t.Fatalf("soak=%d barrier=%d verdict=%q, want %d unresolved", soak, barrier, report.Verdict, want)
			}
		})
	}
}

func TestSoakReportCountsOnlyAwaitingVerificationsOfSucceededCommands(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		verification string
		want         uint64
	}{
		{actionport.VerificationAwaiting, 1},
		{actionport.VerificationReconciled, 0},
		{actionport.VerificationObserved, 0},
	} {
		t.Run(tc.verification, func(t *testing.T) {
			t.Parallel()
			db, _ := openSoakDB(t)
			insertCommand(t, db, "cmd-1", actionport.CommandSucceeded)
			if _, err := db.ExecContext(t.Context(), `INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at)
				VALUES ('ver-1', 'intent-missing', 'cmd-1', NULL, ?, '2026-08-29T12:00:00Z')`, tc.verification); err != nil {
				t.Fatal(err)
			}
			report, err := computeSoak(t, db)
			if err != nil {
				t.Fatal(err)
			}
			if report.Diagnostics["awaiting_verification"] != tc.want || report.Diagnostics["unresolved_action_outcomes"] != tc.want || (report.Verdict == "fail") != (tc.want == 1) {
				t.Fatalf("diagnostics=%v verdict=%q, want %d for a %s verification", report.Diagnostics, report.Verdict, tc.want, tc.verification)
			}
		})
	}
}

func TestSoakReportBindsTenantBeforeTheStatusArguments(t *testing.T) {
	t.Parallel()
	db, _ := openSoakDB(t)
	insertCommand(t, db, "cmd-1", actionport.CommandReconciling)
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET tenant_id = 'other-tenant'`); err != nil {
		t.Fatal(err)
	}
	report, err := computeSoak(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Diagnostics["unresolved_action_outcomes"] != 0 || report.Diagnostics["unknown_outcomes"] != 0 {
		t.Fatalf("another tenant's command was counted: %v", report.Diagnostics)
	}
}

func insertCommand(t *testing.T, db *storage.DB, commandID, status string) {
	t.Helper()
	zeroDigest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands
		(command_id, intent_id, tenant_id, effector_route, normalized_target,
		 idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES (?, 'intent-missing', 'tenant', 'safe_stop', 'fan-01', ?, ?, ?, ?, '2026-08-29T12:00:00Z', '2026-08-29T12:00:00Z')`,
		commandID, zeroDigest, []byte("{}"), zeroDigest, status); err != nil {
		t.Fatal(err)
	}
}

func countBarrier(t *testing.T, db *storage.DB, commandID string) int64 {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	count, err := actions.CountUnresolvedOutcomes(t.Context(), tx, []string{commandID})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSoakReportFailsAnyZeroToleranceEventOrIncompleteEvidence(t *testing.T) {
	t.Parallel()
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
	report, err := computeSoak(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.ZeroTolerance.UnsafeOutputCount != 1 || report.EvidenceCompleteness.Ratio != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestSoakReportFailsWithActiveReconciliationBarrier(t *testing.T) {
	t.Parallel()
	db, _ := openSoakDB(t)
	authority := newAuthority(t, db)
	owner := deviceauthority.Owner{Epoch: "epoch-1", Instance: "instance-1"}
	for _, boot := range []string{"boot-A", "boot-B"} { // the reboot requires reconciliation
		if _, err := authority.RecordDeviceState(t.Context(), owner, map[string]any{"device_id": "thermal-01", "boot_id": boot}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := computeSoak(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "fail" || report.Diagnostics["reconciliation_barriers"] != 1 {
		t.Fatalf("report=%+v", report)
	}
}

// computeSoak derives the report for the test tenant in one read transaction.
func computeSoak(t *testing.T, db *storage.DB) (domain.SoakReport, error) {
	t.Helper()
	var report domain.SoakReport
	err := store.New(db).InSnapshot(t.Context(), func(snapshot *store.Snapshot) error {
		evidence, err := snapshot.SafetyEvidence(t.Context(), "tenant")
		report = domain.DeriveSoakReport(evidence)
		return err
	})
	return report, err
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
	db, err := storagetest.Open(t.Context(), t.TempDir()+"/soak.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ""
}
