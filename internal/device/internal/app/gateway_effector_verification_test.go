package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func fanCommand(commandID, mode string) actionport.Command {
	return actionport.Command{
		CommandID: commandID, EffectorRoute: "select_thermal_mode", NormalizedTarget: "fan-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"mode": mode},
	}
}

func indicatorCommandInState(commandID, state string) actionport.Command {
	command := indicatorCommand(commandID)
	command.Payload = map[string]any{"state": state}
	return command
}

func TestVerificationComparesTheObservedOutputWithTheMaterializedCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		output     map[string]any
		command    actionport.Command
		wantStatus string
		wantTarget string
	}{
		{"indicator at the commanded value", map[string]any{"target": "led-01", "operation": "set_led", "value": float64(1000), "energized": true}, indicatorCommandInState("cmd-alert", "alert"), "succeeded", "led-01"},
		{"indicator at another value", map[string]any{"target": "led-01", "operation": "set_led", "value": float64(500), "energized": true}, indicatorCommandInState("cmd-alert", "alert"), "failed", "led-01"},
		{"fan at the commanded duty", map[string]any{"target": "fan-01", "operation": "set_pwm_lease", "value": float64(450), "energized": true}, fanCommand("cmd-fan", "bounded_cooling"), "succeeded", "fan-01"},
		{"fan at another duty", map[string]any{"target": "fan-01", "operation": "set_pwm_lease", "value": float64(300), "energized": true}, fanCommand("cmd-fan", "bounded_cooling"), "failed", "fan-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session, transport, catalog := openThermalSession(t)
			queueState(t, transport, catalog, map[string]any{"current_output": tc.output})
			status, evidence, err := app.NewGatewayEffector(session, catalog).VerifyDeviceCommand(t.Context(), tc.command)
			if err != nil || status != tc.wantStatus {
				t.Fatalf("verification status=%q err=%v, want %q", status, err, tc.wantStatus)
			}
			if evidence["target"] != tc.wantTarget || evidence["evidence_digest"] == nil {
				t.Fatalf("reconciliation evidence = %v", evidence)
			}
		})
	}
}

func TestVerificationDoesNotAcceptABootRollover(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	queueState(t, transport, catalog, map[string]any{
		"boot_id":        "boot-B",
		"current_output": map[string]any{"target": "led-01", "operation": "set_led", "value": float64(500), "energized": true},
	})
	status, _, err := app.NewGatewayEffector(session, catalog).VerifyDeviceCommand(t.Context(), indicatorCommand("cmd-watch"))
	if status != "" || err == nil || !strings.Contains(err.Error(), "device boot changed") {
		t.Fatalf("boot rollover verification status=%q err=%v", status, err)
	}
	if !reconciliationRequired(t, control) {
		t.Fatal("boot rollover must open the reconciliation barrier")
	}
}

func TestVerificationRefusesACommandOutsideTheCatalogBeforeQueryingTheDevice(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	command := indicatorCommand("cmd-1")
	command.EffectorRoute = "open_valve"
	status, _, err := app.NewGatewayEffector(session, catalog).VerifyDeviceCommand(t.Context(), command)
	if status != "" || err == nil || !strings.Contains(err.Error(), "materialize serial command for verification") {
		t.Fatalf("uncataloged verification status=%q err=%v", status, err)
	}
	if transport.stateQueryCount() != 0 {
		t.Fatalf("verification queried the device %d times", transport.stateQueryCount())
	}
}
