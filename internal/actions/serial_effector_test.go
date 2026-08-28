package actions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
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
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want 1", transport.sendCount())
	}
	sent, err := actions.DecodeDeviceRecord(transport.sentFrames[0])
	if err != nil || sent["target"] != "led-01" || sent["operation"] != "set_led" {
		t.Fatalf("sent device command=%v err=%v", sent, err)
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
}

func TestSerialEffectorConvertsAmbiguousReceiptToUnknownOutcome(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
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
