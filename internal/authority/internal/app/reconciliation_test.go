package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

func TestRebootRequiresReconciliationAcrossRestartAndManualReview(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, required := f.recordState(t, bootA); required {
		t.Fatal("first state required reconciliation")
	}
	state, required := f.recordState(t, bootB)
	if !required {
		t.Fatal("reboot did not require reconciliation")
	}
	_, restarted := f.admit(t, domain.Owner{Epoch: ownerOne.Epoch, Instance: ownerOne.Instance}, runtimeLease)
	if required, err := restarted.ReconciliationRequired(t.Context(), bootB.DeviceID); err != nil || !required {
		t.Fatalf("restart lost the reconciliation: %v, %v", required, err)
	}
	evidence := evidenceFor(t, state, "fan-01")
	if cleared, err := f.service.ResolveReconciliation(t.Context(), domain.ResolutionRequest{Device: bootB, Owner: ownerOne, Outcome: domain.ResolutionManualReview, Evidence: evidence}); err != nil || cleared {
		t.Fatalf("manual review cleared=%v err=%v", cleared, err)
	}
	if cleared, err := f.service.ResolveReconciliation(t.Context(), domain.ResolutionRequest{Device: bootB, Owner: ownerOne, Outcome: domain.ResolutionSucceeded, Evidence: evidence}); err != nil || !cleared {
		t.Fatalf("success cleared=%v err=%v", cleared, err)
	}
	if _, required := f.recordState(t, domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-C"}); !required {
		t.Fatal("new boot did not require reconciliation")
	}
	if f.count(t, `SELECT COUNT(*) FROM device_reconciliation WHERE last_resolution_status IS NOT NULL`) != 0 {
		t.Fatal("new boot kept the previous boot's resolution")
	}
	if f.count(t, `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'reconciliation_opened' AND json_extract(details_json, '$.reason') = 'device_rebooted'`) != 2 {
		t.Fatal("reboots were not audited with a reason")
	}
}

func TestResolveRefusesWithoutAnOpenReconciliation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	state, _ := f.recordState(t, bootA)
	evidence := evidenceFor(t, state, "fan-01")
	if _, err := f.service.ResolveReconciliation(t.Context(), domain.ResolutionRequest{Device: bootA, Owner: ownerOne, Outcome: domain.ResolutionSucceeded, Evidence: evidence}); !errors.Is(err, domain.ErrNoOpenReconciliation) {
		t.Fatalf("resolve on a clear device = %v", err)
	}
	if _, err := f.service.ResolveReconciliation(t.Context(), domain.ResolutionRequest{Device: bootA, Owner: ownerOne, Outcome: "cleared", Evidence: evidence}); err == nil {
		t.Fatal("unknown outcome was accepted")
	}
	unknown := domain.DeviceBoot{DeviceID: "unknown", BootID: "b"}
	if _, err := f.service.ResolveReconciliation(t.Context(), domain.ResolutionRequest{Device: unknown, Owner: ownerOne, Outcome: domain.ResolutionSucceeded, Evidence: evidence}); err == nil {
		t.Fatal("resolution for a device without state was accepted")
	}
}

func TestOpenReconciliationPersistsItsReason(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	for range 2 {
		if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "device receipt was not trustworthy"}); err != nil {
			t.Fatal(err)
		}
	}
	if required, err := f.service.ReconciliationRequired(t.Context(), bootA.DeviceID); err != nil || !required {
		t.Fatalf("required=%v err=%v", required, err)
	}
	if f.count(t, `SELECT COUNT(*) FROM device_authority_events WHERE json_extract(details_json, '$.reason') = 'device receipt was not trustworthy'`) != 2 {
		t.Fatal("each opening must be audited with its reason")
	}
	if required, err := f.service.ReconciliationRequired(t.Context(), "never-seen"); err != nil || required {
		t.Fatalf("unknown device required=%v err=%v", required, err)
	}
}

func TestOpenReconciliationAfterAuthorityLossUsesThePriorityPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	f.clock.Advance(runtimeLease)
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "gateway outcome is unknown"}); !errors.Is(err, control.ErrRuntimeOwnerBusy) {
		t.Fatalf("ordinary opening after authority loss = %v", err)
	}
	if err := f.service.OpenReconciliationAfterAuthorityLoss(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "gateway outcome is unknown"}); err != nil {
		t.Fatalf("priority opening: %v", err)
	}
	if required, err := f.service.ReconciliationRequired(t.Context(), bootA.DeviceID); err != nil || !required {
		t.Fatalf("required=%v err=%v", required, err)
	}
}

func TestOpeningRollsBackWithoutItsAudit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootA)
	if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER reject_opening_audit
		BEFORE INSERT ON device_authority_events WHEN NEW.event_type = 'reconciliation_opened'
		BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "unknown receipt"}); err == nil || !strings.Contains(err.Error(), "record authority event") {
		t.Fatalf("opening failure = %v", err)
	}
	if required, err := f.service.ReconciliationRequired(t.Context(), bootA.DeviceID); err != nil || required {
		t.Fatalf("reconciliation opened without its audit: required=%v err=%v", required, err)
	}
}

func TestOpeningRefusesAPreviousBootOrAnUnknownDevice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.recordState(t, bootB)
	if err := f.service.OpenReconciliationAfterAuthorityLoss(t.Context(), domain.ReconciliationOpening{Device: bootA, Owner: ownerOne, Reason: "unknown receipt"}); err == nil || !strings.Contains(err.Error(), "does not match current device boot") {
		t.Fatalf("previous boot = %v", err)
	}
	unknown := domain.DeviceBoot{DeviceID: "unknown", BootID: "b"}
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: unknown, Owner: ownerOne, Reason: "reason"}); !errors.Is(err, domain.ErrNoRecordedState) {
		t.Fatalf("unknown device = %v", err)
	}
	if err := f.service.OpenReconciliation(t.Context(), domain.ReconciliationOpening{Device: bootB, Owner: ownerOne, Reason: ""}); err == nil {
		t.Fatal("opening without a reason was accepted")
	}
	if f.count(t, `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'reconciliation_opened'`) != 0 {
		t.Fatal("refused openings were audited")
	}
}

func TestADeviceStateWithoutADeviceAndBootIsRefusedAndRecordsNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for name, document := range map[string]map[string]any{
		"no identity":  {"safe_state": true},
		"no boot":      {"device_id": "thermal-01"},
		"not encoded":  {"device_id": "thermal-01", "boot_id": "b", "bad": func() {}},
		"empty device": {"device_id": "", "boot_id": "b"},
	} {
		if _, err := f.service.RecordDeviceState(t.Context(), ownerOne, document); err == nil {
			t.Errorf("%s: the device state was accepted", name)
		}
	}
	if recorded := f.count(t, `SELECT COUNT(*) FROM device_reconciliation`); recorded != 0 {
		t.Fatalf("refused states left %d reconciliation rows", recorded)
	}
}

func TestAskingWhetherADeviceRequiresReconciliationNeedsADeviceId(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if required, err := f.service.ReconciliationRequired(t.Context(), ""); err == nil || required {
		t.Fatalf("ReconciliationRequired(\"\") = %t, %v; want a refusal", required, err)
	}
}
