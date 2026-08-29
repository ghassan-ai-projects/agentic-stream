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
