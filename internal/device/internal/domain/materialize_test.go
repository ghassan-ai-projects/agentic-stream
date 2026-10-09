package domain_test

import (
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestMaterializeProducesBoundedSchemaValidDeviceCommands(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	cases := []struct {
		name          string
		command       actionport.Command
		wantTarget    string
		wantOperation string
		wantParams    map[string]any
	}{
		{"indicator alert", request("set_indicator", "led-01", map[string]any{"entity_id": "zone-01", "state": "alert"}), "led-01", "set_led",
			map[string]any{"brightness_permille": float64(1000), "pattern": "solid"}},
		{"logical target resolves through the closed binding", request("set_indicator", "zone-01", map[string]any{"state": "watch"}), "led-01", "set_led",
			map[string]any{"brightness_permille": float64(500), "pattern": "slow_blink"}},
		{"bounded cooling", request("select_thermal_mode", "fan-01", map[string]any{"entity_id": "zone-01", "mode": "bounded_cooling"}), "fan-01", "set_pwm_lease",
			map[string]any{"duty_permille": float64(450), "lease_ms": float64(5000)}},
		{"fan through its logical target", request("select_thermal_mode", "zone-01", map[string]any{"mode": "hold"}), "fan-01", "set_pwm_lease",
			map[string]any{"duty_permille": float64(0), "lease_ms": float64(5000)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document, err := documentOf(catalog.Materialize(tc.command, bootID))
			if err != nil {
				t.Fatalf("materialize: %v", err)
			}
			if document["target"] != tc.wantTarget || document["operation"] != tc.wantOperation {
				t.Fatalf("target/operation = %v/%v, want %s/%s", document["target"], document["operation"], tc.wantTarget, tc.wantOperation)
			}
			if !reflect.DeepEqual(document["parameters"], tc.wantParams) {
				t.Fatalf("parameters = %#v, want %#v", document["parameters"], tc.wantParams)
			}
			if document["expected_boot_id"] != bootID || document["policy_digest"] != policyKey() || document["not_before_mono_us"] != int64(0) {
				t.Fatalf("boot, policy and freshness binding = %v/%v/%v", document["expected_boot_id"], document["policy_digest"], document["not_before_mono_us"])
			}
			if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
				t.Fatalf("emitted command must validate: %v", err)
			}
		})
	}
}

func TestMaterializeIgnoresModelSuppliedParameters(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	payload := map[string]any{"mode": "bounded_cooling", "duty_permille": float64(999), "lease_ms": float64(99999), "target": "pump-01"}
	document, err := documentOf(catalog.Materialize(request("select_thermal_mode", "fan-01", payload), bootID))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	parameters, _ := document["parameters"].(map[string]any)
	if document["target"] != "fan-01" || parameters["duty_permille"] != float64(450) || parameters["lease_ms"] != float64(5000) {
		t.Fatalf("payload injection reached the device command: target=%v parameters=%v", document["target"], parameters)
	}
}

func TestMaterializeFailsClosed(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	indicator := func(target string) actionport.Command {
		return request("set_indicator", target, map[string]any{"state": "off"})
	}
	withoutIdentity := func(mutate func(*actionport.Command)) actionport.Command {
		command := indicator("led-01")
		mutate(&command)
		return command
	}
	cases := []struct {
		name    string
		command actionport.Command
		boot    string
		want    string
	}{
		{"route not in catalog", request("raise_manual_review", "", map[string]any{"mode": "hold"}), bootID, "is not in the device capability catalog"},
		{"empty target", indicator(""), bootID, "normalized target is required"},
		{"unknown target", indicator("led-99"), bootID, "is not bound to catalog target"},
		{"target of another route", indicator("fan-01"), bootID, "is not bound to catalog target"},
		{"logical target is not bound", indicator("zone-99"), bootID, "is not bound to catalog target"},
		{"selector not an allowed preset", request("select_thermal_mode", "fan-01", map[string]any{"mode": "turbo"}), bootID, "is not an allowed preset"},
		{"missing selector field", request("select_thermal_mode", "fan-01", map[string]any{"entity_id": "zone-01"}), bootID, "requires a string selector"},
		{"selector that is not a string", request("select_thermal_mode", "fan-01", map[string]any{"mode": 3}), bootID, "requires a string selector"},
		{"empty boot id", indicator("led-01"), "", "expected boot id is required"},
		{"no command id", withoutIdentity(func(c *actionport.Command) { c.CommandID = "" }), bootID, "command identity and policy digest are required"},
		{"no idempotency key", withoutIdentity(func(c *actionport.Command) { c.IdempotencyKey = "" }), bootID, "command identity and policy digest are required"},
		{"no policy digest", withoutIdentity(func(c *actionport.Command) { c.PolicyDigest = "" }), bootID, "command identity and policy digest are required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			command, err := catalog.Materialize(tc.command, tc.boot)
			assertRefusal(t, err, tc.want)
			if !reflect.DeepEqual(command, domain.Command{}) {
				t.Fatalf("a refused request emitted a command: %+v", command)
			}
		})
	}
	t.Run("missing catalog", func(t *testing.T) {
		t.Parallel()
		var missing *domain.CapabilityCatalog
		_, err := missing.Materialize(indicator("led-01"), bootID)
		assertRefusal(t, err, "capability catalog is required")
	})
}

func TestMaterializeReEnforcesHardBoundsOnPresetsMisauthoredAboveThem(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		preset string
		bound  string
		want   string
	}{
		{"above the hard max", `{"duty_permille": 900}`, `{"max": 600}`, "exceeds hard max 600"},
		{"below the hard min", `{"duty_permille": 10}`, `{"min": 100}`, "below hard min 100"},
		{"not numeric", `{"duty_permille": "high"}`, `{"max": 600}`, "is not numeric and cannot be bounded"},
		{"at the hard max", `{"duty_permille": 600}`, `{"max": 600}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog, err := domain.LoadCapabilityCatalog([]byte(`{"protocol_version": 1, "routes": {"select_thermal_mode": {
				"operation": "set_pwm_lease", "target": "fan-01", "selector_field": "mode", "expires_after_ms": 20000,
				"presets": {"bounded_cooling": ` + tc.preset + `}, "bounds": {"duty_permille": ` + tc.bound + `}}}}`))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			_, err = catalog.Materialize(request("select_thermal_mode", "fan-01", map[string]any{"mode": "bounded_cooling"}), bootID)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("a preset on its hard bound was refused: %v", err)
				}
				return
			}
			assertRefusal(t, err, tc.want)
		})
	}
}

func TestMaterializeSafeStopIsFixedByTheCatalogAndBoundToItsDigest(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	for _, target := range []string{"led-01", "fan-01"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			first, err := catalog.MaterializeSafeStop(target, "boot-cross")
			if err != nil {
				t.Fatalf("materialize safe stop: %v", err)
			}
			document := first.Document()
			if document["command_id"] != "safe-stop/"+target || document["operation"] != "safe_stop" || document["target"] != target ||
				document["policy_digest"] != thermalCapabilityCatalogHash || document["expected_boot_id"] != "boot-cross" {
				t.Fatalf("safe stop document = %v", document)
			}
			if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
				t.Fatalf("safe stop must validate: %v", err)
			}
			again, _ := catalog.MaterializeSafeStop(target, "boot-cross")
			otherBoot, _ := catalog.MaterializeSafeStop(target, "boot-other")
			if first.IdempotencyKey != again.IdempotencyKey || first.IdempotencyKey == otherBoot.IdempotencyKey {
				t.Fatalf("safe stop identity must repeat on one boot and differ across boots: %q %q %q", first.IdempotencyKey, again.IdempotencyKey, otherBoot.IdempotencyKey)
			}
		})
	}
}

func TestMaterializeSafeStopRefusesWhatTheCatalogDoesNotOwn(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	var missing *domain.CapabilityCatalog
	_, nilErr := missing.MaterializeSafeStop("led-01", bootID)
	_, bootErr := catalog.MaterializeSafeStop("led-01", "")
	_, targetErr := catalog.MaterializeSafeStop("pump-01", bootID)
	assertRefusal(t, nilErr, "capability catalog is required")
	assertRefusal(t, bootErr, "expected boot id is required")
	assertRefusal(t, targetErr, "is not in the capability catalog")
}
