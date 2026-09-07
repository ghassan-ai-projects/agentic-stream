package actions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func TestSerialEffectorReturnsPendingReceiptAfterAuthorization(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	effector := actions.NewSerialEffector(session, catalog)
	checked := false
	effect, err := effector.DispatchAuthorized(context.Background(), actions.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}, actions.Authorization{Check: func(context.Context) error {
		checked = true
		return nil
	}})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !checked || !effect.VerificationPending || effect.ObservedEffect != nil {
		t.Fatalf("authorization/verification state checked=%v pending=%v observed=%v", checked, effect.VerificationPending, effect.ObservedEffect)
	}
	receipt, ok := effect.ProviderResult["receipt"].(map[string]any)
	if !ok || receipt["accepted"] != true {
		t.Fatalf("provider receipt=%#v", effect.ProviderResult)
	}
	result, ok := effect.ProviderResult["result"].(map[string]any)
	if !ok || result["status"] != "executed" {
		t.Fatalf("provider result=%#v", effect.ProviderResult)
	}
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want 1", transport.sendCount())
	}
	sent, err := actions.DecodeDeviceRecord(transport.sentFrames[0])
	if err != nil || sent["target"] != "led-01" || sent["operation"] != "set_led" {
		t.Fatalf("sent device command=%v err=%v", sent, err)
	}
}

func TestSerialEffectorVerificationRejectsMismatchedIndicatorValue(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = digest
	state["current_output"] = map[string]any{
		"target": "led-01", "operation": "set_led", "value": float64(500), "energized": true,
	}
	frame, err := actions.EncodeDeviceRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, frame)

	status, evidence, err := actions.NewSerialEffector(session, catalog).VerifyDeviceCommand(context.Background(), actions.Command{
		CommandID: "cmd-alert", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "alert"},
	})
	if err != nil {
		t.Fatalf("verify device command: %v", err)
	}
	if status != "failed" {
		t.Fatalf("verification status=%q, want failed for a mismatched indicator value", status)
	}
	if evidence["target"] != "led-01" {
		t.Fatalf("reconciliation evidence target=%v, want led-01", evidence["target"])
	}
	if err := storage.ValidateDeviceReconciliationEvidence(evidence, "thermal-01", "boot-A"); err != nil {
		t.Fatalf("query-state evidence must pass durable validation: %v", err)
	}
}

func TestSerialEffectorVerificationDoesNotAcceptBootRollover(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = digest
	state["boot_id"] = "boot-B"
	state["current_output"] = map[string]any{
		"target": "led-01", "operation": "set_led", "value": float64(500), "energized": true,
	}
	frame, err := actions.EncodeDeviceRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, frame)

	status, _, err := actions.NewSerialEffector(session, catalog).VerifyDeviceCommand(context.Background(), actions.Command{
		CommandID: "cmd-watch", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil || status != "" {
		t.Fatalf("boot rollover verification status=%q err=%v, want unresolved error", status, err)
	}
	if !session.ReconciliationRequired() {
		t.Fatal("boot rollover must open the reconciliation barrier")
	}
}

func TestSerialEffectorRejectsBeforeSendAndDoesNotBypassAuthorization(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("unused"))
	defer func() { _ = session.Close() }()
	effector := actions.NewSerialEffector(session, catalog)
	if _, err := effector.DispatchAuthorized(context.Background(), actions.Command{
		CommandID: "cmd-1", EffectorRoute: "select_thermal_mode", NormalizedTarget: "fan-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"mode": "not-a-mode"},
	}, actions.Authorization{Check: func(context.Context) error { return errors.New("interlock tripped") }}); err == nil {
		t.Fatal("unauthorized command was accepted")
	}
	if transport.sendCount() != 0 {
		t.Fatalf("rejected command sent %d frames", transport.sendCount())
	}
}

func TestSerialEffectorReturnsOrdinaryErrorForDeviceRejection(t *testing.T) {
	receipt := acceptedReceipt("cmd-1")
	receipt["accepted"] = false
	receipt["reject_code"] = "expired"
	session, transport, catalog := openThermalSession(t, receipt)
	defer func() { _ = session.Close() }()
	effect, err := actions.NewSerialEffector(session, catalog).Dispatch(context.Background(), actions.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil || actions.IsUnknownOutcome(err) {
		t.Fatalf("device rejection err=%v", err)
	}
	if effect.VerificationPending {
		t.Fatal("rejected device command is pending verification")
	}
	if effect.ProviderResult == nil || transport.sendCount() != 1 {
		t.Fatalf("rejection evidence=%#v sends=%d", effect.ProviderResult, transport.sendCount())
	}
	result, ok := effect.ProviderResult["result"].(map[string]any)
	if !ok || result["status"] != "rejected" || result["error_code"] != "expired" {
		t.Fatalf("rejection result=%#v", effect.ProviderResult)
	}
}

func TestSerialEffectorPreservesReceiptWhenResultIsUntrustworthy(t *testing.T) {
	session, transport, catalog, control := openThermalSessionWithControl(t)
	defer func() { _ = session.Close() }()
	receipt := acceptedReceipt("cmd-result-bad")
	receiptFrame, err := actions.EncodeDeviceRecord(receipt)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, receiptFrame, []byte("{"))

	effect, err := actions.NewSerialEffector(session, catalog).Dispatch(context.Background(), actions.Command{
		CommandID: "cmd-result-bad", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("untrustworthy result err=%v", err)
	}
	receivedReceipt, ok := effect.ProviderResult["receipt"].(map[string]any)
	if !ok || receivedReceipt["command_id"] != "cmd-result-bad" {
		t.Fatalf("partial provider evidence=%#v", effect.ProviderResult)
	}
	if result, ok := effect.ProviderResult["result"].(map[string]any); ok && result != nil {
		t.Fatalf("untrusted result must remain unavailable: %#v", effect.ProviderResult)
	}
	if !transport.closed || !session.ReconciliationRequired() {
		t.Fatalf("invalid result must close transport and require reconciliation closed=%v barrier=%v", transport.closed, session.ReconciliationRequired())
	}
	if _, sent, nextErr := session.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-after-bad-result", idemKey())); nextErr == nil || sent {
		t.Fatalf("session reused after invalid result sent=%v err=%v", sent, nextErr)
	}
	required, controlErr := control.reconciliation.Required(context.Background(), "thermal-01")
	if controlErr != nil || !required {
		t.Fatalf("invalid result did not persist reconciliation barrier required=%v err=%v", required, controlErr)
	}
}

func TestSerialEffectorSafeStopRejectionPreservesKnownEvidence(t *testing.T) {
	receipt := acceptedReceipt("safe-stop/fan-01")
	receipt["accepted"] = false
	receipt["reject_code"] = "not_ready"
	session, _, catalog := openThermalSession(t, receipt)
	defer func() { _ = session.Close() }()

	effect, err := actions.NewSerialEffector(session, catalog).SafeStop(context.Background(), "fan-01")
	if err == nil || actions.IsUnknownOutcome(err) {
		t.Fatalf("known safe-stop rejection err=%v", err)
	}
	if effect.ProviderResult == nil || effect.VerificationPending {
		t.Fatalf("safe-stop rejection evidence=%#v pending=%v", effect.ProviderResult, effect.VerificationPending)
	}
	result, ok := effect.ProviderResult["result"].(map[string]any)
	if !ok || result["status"] != "rejected" || result["error_code"] != "not_ready" {
		t.Fatalf("safe-stop rejection result=%#v", effect.ProviderResult)
	}
	if _, sent, ordinaryErr := session.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-after-safe-stop-rejection", idemKey())); ordinaryErr == nil || sent {
		t.Fatalf("ordinary command crossed latched safe-stop sent=%v err=%v", sent, ordinaryErr)
	}
}

func TestSerialEffectorSafeStopReceiveFailureInvalidatesTransport(t *testing.T) {
	session, transport, catalog, control := openThermalSessionWithControl(t)
	defer func() { _ = session.Close() }()
	transport.receiveErr = errors.New("safe-stop receipt timeout")

	_, err := actions.NewSerialEffector(session, catalog).SafeStop(context.Background(), "fan-01")
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("safe-stop receive failure err=%v", err)
	}
	if !transport.closed {
		t.Fatal("safe-stop receive failure left transport open")
	}
	required, controlErr := control.reconciliation.Required(context.Background(), "thermal-01")
	if controlErr != nil || !required {
		t.Fatalf("safe-stop receive failure barrier required=%v err=%v", required, controlErr)
	}
	if _, sent, nextErr := session.SafeStop(context.Background(), "fan-01"); nextErr == nil || sent {
		t.Fatalf("safe-stop retried on invalidated transport sent=%v err=%v", sent, nextErr)
	}
	if transport.sendCount() != 1 {
		t.Fatalf("safe-stop sends=%d, want 1", transport.sendCount())
	}
}

func TestSerialEffectorPreservesSafeStopReceiptWhenResultIsUntrustworthy(t *testing.T) {
	session, transport, catalog, control := openThermalSessionWithControl(t)
	defer func() { _ = session.Close() }()
	receipt := acceptedReceipt("safe-stop/fan-01")
	receiptFrame, err := actions.EncodeDeviceRecord(receipt)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, receiptFrame, []byte("{"))

	effect, err := actions.NewSerialEffector(session, catalog).SafeStop(context.Background(), "fan-01")
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("untrustworthy safe-stop result err=%v", err)
	}
	receivedReceipt, ok := effect.ProviderResult["receipt"].(map[string]any)
	if !ok || receivedReceipt["command_id"] != "safe-stop/fan-01" {
		t.Fatalf("partial safe-stop provider evidence=%#v", effect.ProviderResult)
	}
	if result, ok := effect.ProviderResult["result"].(map[string]any); ok && result != nil {
		t.Fatalf("untrusted safe-stop result must remain unavailable: %#v", effect.ProviderResult)
	}
	if !transport.closed || !session.ReconciliationRequired() {
		t.Fatalf("untrustworthy safe-stop result closed=%v barrier=%v", transport.closed, session.ReconciliationRequired())
	}
	if required, controlErr := control.reconciliation.Required(context.Background(), "thermal-01"); controlErr != nil || !required {
		t.Fatalf("untrustworthy safe-stop result barrier required=%v err=%v", required, controlErr)
	}
}

func TestSerialEffectorSafeStopRejectionWithUndurableEvidenceIsUnknown(t *testing.T) {
	receipt := acceptedReceipt("safe-stop/fan-01")
	receipt["accepted"] = false
	receipt["reject_code"] = "not_ready"
	session, transport, catalog, control := openThermalSessionWithControl(t, receipt)
	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport.sendHook = cancel
	effect, err := actions.NewSerialEffector(session, catalog).SafeStop(ctx, "fan-01")
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("undurable safe-stop rejection err=%v", err)
	}
	result, ok := effect.ProviderResult["result"].(map[string]any)
	if !ok || result["status"] != "rejected" || result["error_code"] != "not_ready" {
		t.Fatalf("undurable safe-stop rejection evidence=%#v", effect.ProviderResult)
	}
	if !transport.closed || !session.ReconciliationRequired() {
		t.Fatalf("undurable safe-stop rejection closed=%v barrier=%v", transport.closed, session.ReconciliationRequired())
	}
	var failedEvents int
	if err := control.authority.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'safe_stop_failed'`).Scan(&failedEvents); err != nil {
		t.Fatal(err)
	}
	if failedEvents != 0 {
		t.Fatalf("undurable rejection unexpectedly recorded %d safe-stop failure events", failedEvents)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	restartedState := goldenDeviceState()
	restartedState["capability_digest"] = digest
	restartedTransport := &fakeDeviceTransport{frames: mustDeviceFrames(t, restartedState)}
	restarted, err := actions.OpenDeviceSession(context.Background(), actions.DeviceSessionConfig{
		Transport: restartedTransport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: control.reconciliation,
	})
	if err != nil {
		t.Fatalf("restart after undurable safe-stop rejection: %v", err)
	}
	defer func() { _ = restarted.Close() }()
	if _, sent, restartErr := restarted.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-after-undurable-safe-stop", idemKey())); restartErr == nil || sent {
		t.Fatalf("restart crossed safe-stop/barrier sent=%v err=%v", sent, restartErr)
	}
}

func TestSerialEffectorConvertsAmbiguousReceiptToUnknownOutcome(t *testing.T) {
	session, transport, catalog, control := openThermalSessionWithControl(t)
	defer func() { _ = session.Close() }()
	transport.frames = append(transport.frames, []byte("{\"message_type\":\"receipt\"}"))
	_, err := actions.NewSerialEffector(session, catalog).Dispatch(context.Background(), actions.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("malformed post-send receipt err=%v", err)
	}
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want 1", transport.sendCount())
	}
	if !session.ReconciliationRequired() {
		t.Fatal("ambiguous receipt did not open the reconciliation barrier")
	}
	if _, err := session.ResolveReconciliation(context.Background(), "succeeded", map[string]any{"state_digest": "stale"}); err == nil {
		t.Fatal("ambiguous receipt allowed reconciliation without a fresh state query")
	}
	required, err := control.reconciliation.Required(context.Background(), "thermal-01")
	if err != nil || !required {
		t.Fatalf("ambiguous receipt did not persist reconciliation barrier required=%v err=%v", required, err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest catalog after restart: %v", err)
	}
	restartedState := goldenDeviceState()
	restartedState["capability_digest"] = digest
	restartedTransport := &fakeDeviceTransport{frames: mustDeviceFrames(t, restartedState)}
	restarted, err := actions.OpenDeviceSession(context.Background(), actions.DeviceSessionConfig{
		Transport: restartedTransport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
		AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: control.authority, Reconciliation: control.reconciliation,
	})
	if err != nil {
		t.Fatalf("restart after ambiguous receipt: %v", err)
	}
	defer func() { _ = restarted.Close() }()
	if _, sent, err := restarted.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-after-restart", idemKey())); err == nil || sent {
		t.Fatalf("ordinary command crossed durable ambiguity barrier after restart sent=%v err=%v", sent, err)
	}
}

func TestSerialEffectorPersistsBarrierAfterReceiptContextCancellation(t *testing.T) {
	session, transport, catalog, control := openThermalSessionWithControl(t)
	defer func() { _ = session.Close() }()
	transport.receiveErr = context.Canceled
	ctx, cancel := context.WithCancel(context.Background())
	transport.sendHook = cancel

	_, err := actions.NewSerialEffector(session, catalog).Dispatch(ctx, actions.Command{
		CommandID: "cmd-canceled", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("canceled receipt err=%v", err)
	}
	required, err := control.reconciliation.Required(context.Background(), "thermal-01")
	if err != nil || !required {
		t.Fatalf("canceled receipt did not persist reconciliation barrier required=%v err=%v", required, err)
	}
}

func TestSerialEffectorSafeStopUnknownOutcomeOpensReconciliationBarrier(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	transport.frames = append(transport.frames, []byte("{"))

	_, err := actions.NewSerialEffector(session, catalog).SafeStop(context.Background(), "fan-01")
	if err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("malformed safe-stop receipt err=%v", err)
	}
	if !session.ReconciliationRequired() {
		t.Fatal("unknown safe-stop outcome did not open the reconciliation barrier")
	}
}

func TestSerialEffectorExportsPendingUnknownAndFrameMetrics(t *testing.T) {
	metrics := telemetry.NewRuntime(time.Unix(1, 0))
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	effector := actions.NewSerialEffector(session, catalog).WithTelemetry(metrics)
	command := actions.Command{CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01", IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"}}
	if _, err := effector.Dispatch(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, []byte("{"))
	command.CommandID = "cmd-2"
	command.IdempotencyKey = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	if _, err := effector.Dispatch(context.Background(), command); err == nil || !actions.IsUnknownOutcome(err) {
		t.Fatalf("malformed receipt err=%v", err)
	}
	snapshot := metrics.Snapshot()
	if snapshot["agentic_stream_verification_pending_total"] != 1 || snapshot["agentic_stream_action_unknown_outcomes_total"] != 1 || snapshot["agentic_stream_device_frame_errors_total"] != 1 {
		t.Fatalf("action metrics=%v", snapshot)
	}
}

func TestSerialEffectorDuplicateUsesSessionReceiptCache(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	effector := actions.NewSerialEffector(session, catalog)
	command := actions.Command{CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01", IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"}}
	first, err := effector.Dispatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	command.CommandID = "cmd-2"
	second, err := effector.Dispatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	firstReceipt := first.ProviderResult["receipt"].(map[string]any)
	secondReceipt := second.ProviderResult["receipt"].(map[string]any)
	if firstReceipt["command_id"] != secondReceipt["command_id"] || transport.sendCount() != 1 {
		t.Fatalf("duplicate receipts first=%v second=%v sends=%d", first, second, transport.sendCount())
	}
}

func TestSerialEffectorRejectsCatalogDifferentFromHandshake(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	other := loadThermalCatalog(t)
	spec := other.Routes["set_indicator"]
	spec.Operation = "different_led_operation"
	other.Routes["set_indicator"] = spec
	_, err := actions.NewSerialEffector(session, other).Dispatch(context.Background(), actions.Command{
		CommandID: "cmd-1", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err == nil {
		t.Fatal("catalog mismatch was accepted")
	}
	if transport.sendCount() != 0 || catalog == other {
		t.Fatalf("catalog mismatch crossed transport: sends=%d same=%v", transport.sendCount(), catalog == other)
	}
}
