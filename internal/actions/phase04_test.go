package actions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSerialSessionBootBarrierSurvivesAndClearsOnlyWithBoundState(t *testing.T) {
	control := newDeviceControl(t)
	db := control.authority.DB
	store := control.reconciliation
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	initial := goldenDeviceState()
	initial["capability_digest"] = digest
	bootB := goldenDeviceState()
	bootB["capability_digest"] = digest
	bootB["boot_id"] = "boot-B"
	transport := &fakeDeviceTransport{frames: mustDeviceFrames(t, initial)}
	session, err := actions.OpenDeviceSession(t.Context(), actions.DeviceSessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{initial["firmware_digest"].(string)},
		AuthorityEpoch:         "epoch-1", OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	transport.frames = append(transport.frames, mustDeviceFrames(t, bootB)...)
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatal(err)
	}
	if transport.stateQueries != 1 {
		t.Fatalf("state queries=%d, want 1", transport.stateQueries)
	}
	command := materializedCommandWithBoot(t, catalog, "cmd-barrier", idemKey(), "boot-B")
	if _, sent, err := session.Exchange(t.Context(), command); err == nil || sent {
		t.Fatalf("barrier allowed ordinary command sent=%v err=%v", sent, err)
	}
	evidence := reconciliationEvidence(t, bootB, "fan-01")
	cleared, err := session.ResolveReconciliation(t.Context(), "succeeded", evidence)
	if err != nil || !cleared {
		t.Fatalf("resolve barrier cleared=%v err=%v", cleared, err)
	}
	receipt := acceptedReceipt("cmd-barrier")
	receipt["boot_id"] = "boot-B"
	transport.frames = append(transport.frames, mustDeviceFrames(t, receipt)...)
	if _, sent, err := session.Exchange(t.Context(), command); err != nil || !sent {
		t.Fatalf("cleared barrier did not allow command sent=%v err=%v", sent, err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM device_reconciliation WHERE device_id = 'thermal-01'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "clear" {
		t.Fatalf("stored barrier status=%q, want clear", status)
	}
}

func TestSerialSessionSafeStopWinsOverBarrierAndRejectsLaterOrdinaryWork(t *testing.T) {
	control := newDeviceControl(t)
	db := control.authority.DB
	store := control.reconciliation
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = digest
	safeReceipt := acceptedReceipt("safe-stop/fan-01")
	safeReceipt["boot_id"] = "boot-B"
	bootB := goldenDeviceState()
	bootB["capability_digest"] = digest
	bootB["boot_id"] = "boot-B"
	transport := &fakeDeviceTransport{frames: append(mustDeviceFrames(t, state), mustDeviceFrames(t, bootB, safeReceipt)...)}
	session, err := actions.OpenDeviceSession(t.Context(), actions.DeviceSessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{state["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatal(err)
	}
	effector := actions.NewSerialEffector(session, catalog)
	if _, err := effector.SafeStop(t.Context(), "fan-01"); err != nil {
		t.Fatalf("safe stop behind barrier failed: %v", err)
	}
	if _, sent, err := session.Exchange(t.Context(), materializedCommandWithBoot(t, catalog, "cmd-after-stop", idemKey(), "boot-B")); err == nil || sent {
		t.Fatalf("ordinary command crossed safe stop sent=%v err=%v", sent, err)
	}
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want only the safe stop", transport.sendCount())
	}
	var completed int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'safe_stop_completed'`).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 {
		t.Fatalf("durable safe-stop completion events=%d, want 1", completed)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	restartedTransport := &fakeDeviceTransport{frames: mustDeviceFrames(t, bootB)}
	restarted, err := actions.OpenDeviceSession(t.Context(), actions.DeviceSessionConfig{
		Transport: restartedTransport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{bootB["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	if _, sent, err := restarted.Exchange(t.Context(), materializedCommandWithBoot(t, catalog, "cmd-after-restart", idemKey(), "boot-B")); err == nil || sent {
		t.Fatalf("ordinary command crossed durable safe-stop after restart sent=%v err=%v", sent, err)
	}
}

func TestSerialSessionAuthorityLossAfterTransportIsUnknown(t *testing.T) {
	db := openActionDB(t)
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = digest
	transport := &fakeDeviceTransport{frames: mustDeviceFrames(t, state)}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: 10, Now: func() time.Time { return now }}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	epochControl := &storage.EpochControl{DB: db}
	authority := &storage.TargetAuthority{DB: db, Owner: owner, EpochControl: epochControl, InstanceID: "instance-1", Lease: 10, Now: func() time.Time { return now }}
	store := &storage.ReconciliationStore{DB: db, Authority: authority, Now: func() time.Time { return now }}
	session, err := actions.OpenDeviceSession(t.Context(), actions.DeviceSessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{state["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: authority, Reconciliation: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	receipt := acceptedReceipt("cmd-unknown")
	transport.frames = append(transport.frames, mustDeviceFrames(t, receipt)...)
	transport.receiveHook = func() { now = now.Add(11 * time.Second) }
	if _, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-unknown", idemKey())); err == nil || !sent {
		t.Fatalf("authority loss after transport was not unknown sent=%v err=%v", sent, err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM device_reconciliation WHERE device_id = 'thermal-01'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "required" {
		t.Fatalf("authority loss did not persist reconciliation barrier: %q", status)
	}
}

func TestSerialSessionDisablesAfterReconciliationPersistenceFailure(t *testing.T) {
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = digest
	transport := &fakeDeviceTransport{frames: mustDeviceFrames(t, state)}
	session, err := actions.OpenDeviceSession(t.Context(), actions.DeviceSessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{state["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: control.reconciliation,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if _, err := control.authority.DB.ExecContext(t.Context(), "DROP TABLE device_reconciliation"); err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, mustDeviceFrames(t, state)...)
	if _, err := session.QueryState(t.Context()); err == nil {
		t.Fatal("refresh unexpectedly succeeded after reconciliation store failure")
	}
	if _, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-after-store-failure", idemKey())); err == nil || sent {
		t.Fatalf("session exchanged after reconciliation store failure sent=%v err=%v", sent, err)
	}
	if transport.sendCount() != 0 {
		t.Fatalf("transport sends=%d, want 0", transport.sendCount())
	}
}

func TestSerialSessionStartupBarrierRequiresFreshStateQuery(t *testing.T) {
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	stateA := goldenDeviceState()
	stateA["capability_digest"] = digest
	stateB := goldenDeviceState()
	stateB["capability_digest"] = digest
	stateB["boot_id"] = "boot-B"
	transport1 := &fakeDeviceTransport{frames: append(mustDeviceFrames(t, stateA), mustDeviceFrames(t, stateB)...)}
	config := actions.DeviceSessionConfig{
		Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{stateA["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: control.reconciliation,
	}
	config.Transport = transport1
	session1, err := actions.OpenDeviceSession(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session1.QueryState(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := session1.Close(); err != nil {
		t.Fatal(err)
	}

	transport2 := &fakeDeviceTransport{frames: append(mustDeviceFrames(t, stateB), mustDeviceFrames(t, stateB)...)}
	config.Transport = transport2
	session2, err := actions.OpenDeviceSession(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session2.Close() }()
	if _, err := session2.ResolveReconciliation(t.Context(), "succeeded", reconciliationEvidence(t, stateB, "fan-01")); err == nil {
		t.Fatal("startup barrier resolved without a fresh state query")
	}
	if _, err := session2.QueryState(t.Context()); err != nil {
		t.Fatal(err)
	}
	if cleared, err := session2.ResolveReconciliation(t.Context(), "succeeded", reconciliationEvidence(t, stateB, "fan-01")); err != nil || !cleared {
		t.Fatalf("fresh state query did not permit reconciliation cleared=%v err=%v", cleared, err)
	}
}

func openActionDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), t.TempDir()+"/actions.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustDeviceFrames(t *testing.T, documents ...map[string]any) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, len(documents))
	for _, document := range documents {
		frame, err := actions.EncodeDeviceRecord(document)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func reconciliationEvidence(t *testing.T, state map[string]any, target string) map[string]any {
	t.Helper()
	stateJSON, err := canonicaljson.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	feedback := map[string]any{"target": target, "observed_state": "safe", "observed_at": "2026-08-29T12:00:00Z"}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[string]any{
		"device_id":       state["device_id"],
		"boot_id":         state["boot_id"],
		"evidence_type":   "device_state_feedback",
		"target":          target,
		"state":           state,
		"state_digest":    "sha256:" + sha256Hex(stateJSON),
		"feedback":        feedback,
		"feedback_digest": "sha256:" + sha256Hex(feedbackJSON),
		"source":          "independent-feedback",
	}
	bundle, err := canonicaljson.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidence["evidence_digest"] = "sha256:" + sha256Hex(bundle)
	return evidence
}

func materializedCommandWithBoot(t *testing.T, catalog *actions.CapabilityCatalog, commandID, idempotency, bootID string) map[string]any {
	t.Helper()
	command, err := catalog.Materialize(actions.Command{
		CommandID: commandID, EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idempotency, PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}, bootID)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
