package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

var errInterlockTripped = errors.New("interlock tripped")

func TestGatewayEffectorSendsOnlyAfterAuthorizationAndLeavesVerificationPending(t *testing.T) {
	t.Parallel()
	routes := []struct {
		name          string
		command       actionport.Command
		wantTarget    string
		wantOperation string
	}{
		{"indicator", indicatorCommand("cmd-1"), "led-01", "set_led"},
		{"thermal mode", actionport.Command{
			CommandID: "cmd-1", EffectorRoute: "select_thermal_mode", NormalizedTarget: "fan-01",
			IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"mode": "bounded_cooling"},
		}, "fan-01", "set_pwm_lease"},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			t.Parallel()
			session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
			checked := false
			effect, err := app.NewGatewayEffector(session, catalog).DispatchAuthorized(t.Context(), route.command,
				actionport.Authorization{Check: func(context.Context) error { checked = true; return nil }})
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if !checked || !effect.VerificationPending || effect.ObservedEffect != nil {
				t.Fatalf("checked=%v pending=%v observed=%v", checked, effect.VerificationPending, effect.ObservedEffect)
			}
			receipt, _ := effect.ProviderResult["receipt"].(map[string]any)
			result, _ := effect.ProviderResult["result"].(map[string]any)
			if receipt["accepted"] != true || result["status"] != "executed" {
				t.Fatalf("provider evidence = %#v", effect.ProviderResult)
			}
			sent, err := wire.Decode(transport.firstSentFrame())
			if transport.sendCount() != 1 || err != nil || sent["target"] != route.wantTarget || sent["operation"] != route.wantOperation {
				t.Fatalf("sends=%d sent device command=%v err=%v", transport.sendCount(), sent, err)
			}
		})
	}
}

func TestGatewayEffectorRefusesAnUnauthorizedCommandBeforeMaterializingOrSending(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, acceptedReceipt("unused"))
	command := actionport.Command{
		CommandID: "cmd-1", EffectorRoute: "select_thermal_mode", NormalizedTarget: "fan-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"mode": "not-a-mode"},
	}
	_, err := app.NewGatewayEffector(session, catalog).DispatchAuthorized(t.Context(), command,
		actionport.Authorization{Check: func(context.Context) error { return errInterlockTripped }})
	if !errors.Is(err, errInterlockTripped) {
		t.Fatalf("unauthorized dispatch error = %v, want the interlock refusal", err)
	}
	if transport.sendCount() != 0 {
		t.Fatalf("rejected command sent %d frames", transport.sendCount())
	}
}

func TestGatewayEffectorRefusesACommandOutsideTheCatalogBeforeSending(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	command := indicatorCommand("cmd-1")
	command.EffectorRoute = "open_valve"
	_, err := app.NewGatewayEffector(session, catalog).Dispatch(t.Context(), command)
	if err == nil || actionport.IsUnknownOutcome(err) || !strings.Contains(err.Error(), "materialize serial command") {
		t.Fatalf("uncataloged route error = %v", err)
	}
	if transport.sendCount() != 0 {
		t.Fatalf("uncataloged route sent %d frames", transport.sendCount())
	}
}

func TestGatewayEffectorReturnsAnOrdinaryErrorForADeviceRejection(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, rejectedReceipt("cmd-1", "expired"))
	effect, err := app.NewGatewayEffector(session, catalog).Dispatch(t.Context(), indicatorCommand("cmd-1"))
	if err == nil || actionport.IsUnknownOutcome(err) || !strings.Contains(err.Error(), "device rejected serial command: expired") {
		t.Fatalf("device rejection err=%v", err)
	}
	if effect.VerificationPending || transport.sendCount() != 1 {
		t.Fatalf("pending=%v sends=%d", effect.VerificationPending, transport.sendCount())
	}
	result, _ := effect.ProviderResult["result"].(map[string]any)
	if result["status"] != "rejected" || result["error_code"] != "expired" {
		t.Fatalf("rejection result=%#v", effect.ProviderResult)
	}
}

func TestGatewayEffectorAnswersADuplicateIdempotencyKeyFromTheSessionCache(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	effector := app.NewGatewayEffector(session, catalog)
	first, err := effector.Dispatch(t.Context(), indicatorCommand("cmd-1"))
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	second, err := effector.Dispatch(t.Context(), indicatorCommand("cmd-2"))
	if err != nil {
		t.Fatalf("duplicate dispatch: %v", err)
	}
	firstReceipt, _ := first.ProviderResult["receipt"].(map[string]any)
	secondReceipt, _ := second.ProviderResult["receipt"].(map[string]any)
	if firstReceipt["command_id"] != "cmd-1" || secondReceipt["command_id"] != "cmd-1" || transport.sendCount() != 1 {
		t.Fatalf("duplicate receipts first=%v second=%v sends=%d", first, second, transport.sendCount())
	}
}

func TestGatewayEffectorRefusesACatalogDifferentFromTheHandshakeBeforeSending(t *testing.T) {
	t.Parallel()
	session, transport, _ := openThermalSession(t, acceptedReceipt("cmd-1"))
	other := loadThermalCatalog(t)
	spec := other.Routes["set_indicator"]
	spec.Operation = "different_led_operation"
	other.Routes["set_indicator"] = spec
	_, err := app.NewGatewayEffector(session, other).Dispatch(t.Context(), indicatorCommand("cmd-1"))
	if err == nil || !strings.Contains(err.Error(), "catalog does not match the device session") {
		t.Fatalf("catalog mismatch error = %v", err)
	}
	if transport.sendCount() != 0 {
		t.Fatalf("catalog mismatch sent %d frames", transport.sendCount())
	}
}

func TestAnUnconfiguredGatewayEffectorRefusesEveryOperation(t *testing.T) {
	t.Parallel()
	var missing *app.GatewayEffector
	for name, effector := range map[string]*app.GatewayEffector{
		"nil":                   missing,
		"no session or catalog": app.NewGatewayEffector(nil, nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := effector.Dispatch(t.Context(), indicatorCommand("cmd-1")); err == nil || !strings.Contains(err.Error(), "session and catalog are required") {
				t.Fatalf("dispatch error = %v", err)
			}
			if _, _, err := effector.VerifyDeviceCommand(t.Context(), indicatorCommand("cmd-1")); err == nil || !strings.Contains(err.Error(), "session and catalog are required") {
				t.Fatalf("verify error = %v", err)
			}
			if _, err := effector.SafeStop(t.Context(), "fan-01"); err == nil || !strings.Contains(err.Error(), "session is required") {
				t.Fatalf("safe stop error = %v", err)
			}
		})
	}
}

func TestGatewayEffectorExportsPendingUnknownAndFrameErrorMetrics(t *testing.T) {
	t.Parallel()
	metrics := telemetry.NewRuntime(time.Unix(1, 0))
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	effector := app.NewGatewayEffector(session, catalog).WithTelemetry(metrics)
	if _, err := effector.Dispatch(t.Context(), indicatorCommand("cmd-1")); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	transport.queue([]byte("{"))
	second := indicatorCommand("cmd-2")
	second.IdempotencyKey = "sha256:" + strings.Repeat("d", 64)
	if _, err := effector.Dispatch(t.Context(), second); !actionport.IsUnknownOutcome(err) {
		t.Fatalf("malformed receipt err=%v, want an unknown outcome", err)
	}
	snapshot := metrics.Snapshot()
	for name, want := range map[string]uint64{
		"agentic_stream_verification_pending_total":    1,
		"agentic_stream_action_unknown_outcomes_total": 1,
		"agentic_stream_device_frame_errors_total":     1,
	} {
		if snapshot[name] != want {
			t.Errorf("%s = %v, want %v (snapshot %v)", name, snapshot[name], want, snapshot)
		}
	}
}
