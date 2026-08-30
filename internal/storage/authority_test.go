package storage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestTargetAuthorityFencesConflictingEpochAndRecoversExpiredClaim(t *testing.T) {
	db, _ := openOwnerDB(t)
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	authority1 := &storage.TargetAuthority{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	claim := storage.TargetClaim{Target: "fan-01", DeviceID: "thermal-01", BootID: "boot-A", AuthorityEpoch: "epoch-1", OwnerInstance: "instance-1"}
	if err := authority1.Claim(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	authority2 := &storage.TargetAuthority{DB: db, InstanceID: "instance-2", Lease: time.Minute, Now: func() time.Time { return now.Add(30 * time.Second) }}
	claim2 := storage.TargetClaim{Target: "fan-01", DeviceID: "thermal-01", BootID: "boot-B", AuthorityEpoch: "epoch-2", OwnerInstance: "instance-2"}
	if err := authority2.Claim(t.Context(), claim2); err == nil {
		t.Fatal("unexpired target claim was accepted by a second epoch")
	}
	var eventType string
	if err := db.QueryRowContext(t.Context(), `SELECT event_type FROM device_authority_events WHERE event_type = 'claim_rejected'`).Scan(&eventType); err != nil {
		t.Fatalf("claim rejection event missing: %v", err)
	}
	if eventType != "claim_rejected" {
		t.Fatal(eventType)
	}
	now = now.Add(2 * time.Minute)
	if err := authority2.Claim(t.Context(), claim2); err != nil {
		t.Fatalf("expired claim was not recoverable: %v", err)
	}
	if err := authority2.Assert(t.Context(), claim2); err != nil {
		t.Fatalf("recovered claim did not assert: %v", err)
	}
}

func TestReconciliationStoreKeepsBarrierAcrossRestartAndManualReview(t *testing.T) {
	db, _ := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority := &storage.TargetAuthority{DB: db, Owner: owner, EpochControl: &storage.EpochControl{DB: db}, InstanceID: "instance-1", Lease: time.Minute}
	store := &storage.ReconciliationStore{DB: db, Authority: authority}
	state := map[string]any{"message_type": "state", "device_id": "thermal-01", "boot_id": "boot-A", "safe_state": true}
	required, err := store.BindState(t.Context(), state, "epoch-1", "instance-1")
	if err != nil || required {
		t.Fatalf("initial state required=%v err=%v", required, err)
	}
	state["boot_id"] = "boot-B"
	required, err = store.BindState(t.Context(), state, "epoch-1", "instance-1")
	if err != nil || !required {
		t.Fatalf("boot change required=%v err=%v", required, err)
	}
	required, err = (&storage.ReconciliationStore{DB: db}).Required(t.Context(), "thermal-01")
	if err != nil || !required {
		t.Fatalf("restart lost barrier required=%v err=%v", required, err)
	}
	stateJSON, err := canonicaljson.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateHash := sha256.Sum256(stateJSON)
	feedback := map[string]any{"target": "fan-01", "observed_state": "safe", "observed_at": "2026-08-29T12:00:00Z"}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	evidence := map[string]any{
		"device_id": state["device_id"], "boot_id": state["boot_id"], "state": state,
		"evidence_type": "device_state_feedback",
		"state_digest":  "sha256:" + hex.EncodeToString(stateHash[:]), "source": "independent-feedback",
		"feedback": feedback, "feedback_digest": "sha256:" + hex.EncodeToString(feedbackHash[:]),
	}
	evidenceJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidenceHash := sha256.Sum256(evidenceJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(evidenceHash[:])
	cleared, err := store.Resolve(t.Context(), "thermal-01", "boot-B", "manual_review", evidence, "epoch-1", "instance-1")
	if err != nil || cleared {
		t.Fatalf("manual review cleared=%v err=%v", cleared, err)
	}
	cleared, err = store.Resolve(t.Context(), "thermal-01", "boot-B", "succeeded", evidence, "epoch-1", "instance-1")
	if err != nil || !cleared {
		t.Fatalf("successful reconciliation cleared=%v err=%v", cleared, err)
	}
	state["boot_id"] = "boot-C"
	if required, err := store.BindState(t.Context(), state, "epoch-1", "instance-1"); err != nil || !required {
		t.Fatalf("new boot required=%v err=%v", required, err)
	}
	var resolutionStatus string
	if err := db.QueryRowContext(t.Context(), `SELECT COALESCE(last_resolution_status, '') FROM device_reconciliation WHERE device_id = 'thermal-01'`).Scan(&resolutionStatus); err != nil {
		t.Fatal(err)
	}
	if resolutionStatus != "" {
		t.Fatalf("new boot retained prior resolution status %q", resolutionStatus)
	}
}

func TestReconciliationStorePersistsAmbiguousOutcomeBarrier(t *testing.T) {
	db, _ := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority := &storage.TargetAuthority{DB: db, Owner: owner, EpochControl: &storage.EpochControl{DB: db}, InstanceID: "instance-1", Lease: time.Minute}
	store := &storage.ReconciliationStore{DB: db, Authority: authority}
	state := map[string]any{"device_id": "thermal-01", "boot_id": "boot-A", "safe_state": true}
	if _, err := store.BindState(t.Context(), state, "epoch-1", "instance-1"); err != nil {
		t.Fatalf("bind initial state: %v", err)
	}
	if err := store.Require(t.Context(), "thermal-01", "boot-A", "epoch-1", "instance-1", "device receipt was not trustworthy"); err != nil {
		t.Fatalf("persist ambiguous outcome barrier: %v", err)
	}
	required, err := (&storage.ReconciliationStore{DB: db}).Required(t.Context(), "thermal-01")
	if err != nil || !required {
		t.Fatalf("persisted barrier required=%v err=%v", required, err)
	}
	var reason string
	if err := db.QueryRowContext(t.Context(), `
		SELECT json_extract(details_json, '$.reason')
		FROM device_authority_events WHERE event_type = 'reconciliation_opened'
		ORDER BY event_id DESC LIMIT 1`).Scan(&reason); err != nil {
		t.Fatalf("read reconciliation event: %v", err)
	}
	if reason != "device receipt was not trustworthy" {
		t.Fatalf("reconciliation reason=%q", reason)
	}
}

func TestReconciliationStoreOpensBarrierAfterAuthorityLoss(t *testing.T) {
	db, _ := openOwnerDB(t)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority := &storage.TargetAuthority{DB: db, Owner: owner, EpochControl: &storage.EpochControl{DB: db}, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	store := &storage.ReconciliationStore{DB: db, Authority: authority, Now: func() time.Time { return now }}
	state := map[string]any{"device_id": "thermal-01", "boot_id": "boot-A", "safe_state": true}
	if _, err := store.BindState(t.Context(), state, "epoch-1", "instance-1"); err != nil {
		t.Fatalf("bind initial state: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := store.Require(t.Context(), "thermal-01", "boot-A", "epoch-1", "instance-1", "gateway outcome is unknown"); err == nil {
		t.Fatal("expired authority unexpectedly opened reconciliation normally")
	}
	if err := store.RequireAfterAuthorityLoss(t.Context(), "thermal-01", "boot-A", "epoch-1", "instance-1", "gateway outcome is unknown"); err != nil {
		t.Fatalf("recovery barrier was not durable after authority loss: %v", err)
	}
	required, err := store.Required(t.Context(), "thermal-01")
	if err != nil || !required {
		t.Fatalf("recovery barrier required=%v err=%v", required, err)
	}
}

func TestReconciliationStoreResolvesOneDeviceDespiteAnotherDeviceOutcome(t *testing.T) {
	db, _ := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority := &storage.TargetAuthority{DB: db, Owner: owner, EpochControl: &storage.EpochControl{DB: db}, InstanceID: "instance-1", Lease: time.Minute}
	store := &storage.ReconciliationStore{DB: db, Authority: authority}
	stateA := map[string]any{"device_id": "thermal-a", "boot_id": "boot-a", "safe_state": true}
	stateB := map[string]any{"device_id": "thermal-b", "boot_id": "boot-b", "safe_state": true}
	for _, state := range []map[string]any{stateA, stateB} {
		if _, err := store.BindState(t.Context(), state, "epoch-1", "instance-1"); err != nil {
			t.Fatalf("bind state %v: %v", state["device_id"], err)
		}
	}
	if err := store.Require(t.Context(), "thermal-a", "boot-a", "epoch-1", "instance-1", "device A receipt was not trustworthy"); err != nil {
		t.Fatalf("open device A barrier: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES ('cmd-device-b', 'intent-device-b', 'tenant', 'set_indicator', 'led-b', ?, ?, ?, 'outcome_unknown', ?, ?)`,
		make([]byte, 32), []byte("{}"), make([]byte, 32), "2026-08-30T00:00:00.000000000Z", "2026-08-30T00:00:00.000000000Z"); err != nil {
		t.Fatalf("insert device B command: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO device_command_bindings (
			command_id, target, device_id, boot_id, owner_epoch, owner_instance, bound_at
		) VALUES ('cmd-device-b', 'led-b', 'thermal-b', 'boot-b', 'epoch-1', 'instance-1', ?)`,
		"2026-08-30T00:00:00.000000000Z"); err != nil {
		t.Fatalf("bind device B command: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	evidence := reconciliationEvidence(t, stateA, "led-a")
	cleared, err := store.Resolve(t.Context(), "thermal-a", "boot-a", "succeeded", evidence, "epoch-1", "instance-1")
	if err != nil || !cleared {
		t.Fatalf("device A barrier remained blocked by device B cleared=%v err=%v", cleared, err)
	}
}

func reconciliationEvidence(t *testing.T, state map[string]any, target string) map[string]any {
	t.Helper()
	stateJSON, err := canonicaljson.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateHash := sha256.Sum256(stateJSON)
	feedback := map[string]any{"target": target, "observed_state": "safe", "observed_at": "2026-08-30T00:00:00Z"}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	evidence := map[string]any{
		"device_id": state["device_id"], "boot_id": state["boot_id"], "state": state,
		"evidence_type": "device_state_feedback",
		"state_digest":  "sha256:" + hex.EncodeToString(stateHash[:]), "source": "independent-feedback",
		"feedback": feedback, "feedback_digest": "sha256:" + hex.EncodeToString(feedbackHash[:]),
	}
	evidenceJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidenceHash := sha256.Sum256(evidenceJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(evidenceHash[:])
	return evidence
}

func TestSafetyLedgerRejectsUnknownTypeAndStoresExplicitEvent(t *testing.T) {
	db, _ := openOwnerDB(t)
	ledger := &storage.SafetyLedger{DB: db}
	if err := ledger.Record(t.Context(), storage.SafetyEvent{Type: "unknown", Target: "fan-01"}); err == nil {
		t.Fatal("unknown safety event type was accepted")
	}
	if err := ledger.Record(t.Context(), storage.SafetyEvent{Type: "physical_transition", Target: "fan-01", Details: map[string]any{
		"evidence_complete": true, "source": "independent-feedback",
		"evidence_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM device_safety_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("safety event count=%d err=%v", count, err)
	}
}
