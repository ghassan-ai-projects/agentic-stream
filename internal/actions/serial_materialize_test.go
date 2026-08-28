package actions_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// The deterministic intent→device-command materializer (Real-World Sensor
// HIL-0, Phase 03 Task 3.2). The central property under test: the model can
// never introduce a target, operation, or parameter value — the concrete device
// parameters come only from the closed capability catalog, and hard bounds are
// re-enforced at this last boundary.

const boot = "boot-A"

func loadThermalCatalog(t *testing.T) *actions.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("testdata/thermal_capability_catalog.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	catalog, err := actions.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func TestMaterializeProducesBoundedDeviceCommands(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)

	t.Run("led indicator", func(t *testing.T) {
		t.Parallel()
		doc, err := catalog.Materialize(actions.Command{
			CommandID: "cmd-1", EffectorRoute: "set_indicator", IdempotencyKey: idemKey(),
			Payload: map[string]any{"entity_id": "zone-01", "state": "alert"},
		}, boot)
		if err != nil {
			t.Fatalf("materialize: %v", err)
		}
		if doc["operation"] != "set_led" || doc["target"] != "led-01" {
			t.Fatalf("unexpected op/target: %v / %v", doc["operation"], doc["target"])
		}
		if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, doc); err != nil {
			t.Fatalf("emitted command must validate: %v", err)
		}
	})

	t.Run("bounded cooling mode", func(t *testing.T) {
		t.Parallel()
		doc, err := catalog.Materialize(actions.Command{
			CommandID: "cmd-2", EffectorRoute: "select_thermal_mode", IdempotencyKey: idemKey(),
			Payload: map[string]any{"entity_id": "zone-01", "mode": "bounded_cooling"},
		}, boot)
		if err != nil {
			t.Fatalf("materialize: %v", err)
		}
		params := doc["parameters"].(map[string]any)
		if params["duty_permille"].(float64) != 450 || params["lease_ms"].(float64) != 5000 {
			t.Fatalf("unexpected materialized params: %v", params)
		}
		if doc["expected_boot_id"] != boot {
			t.Fatalf("expected_boot_id = %v, want %s", doc["expected_boot_id"], boot)
		}
	})
}

// The safety-critical property: a payload that tries to smuggle its own device
// parameters is ignored — the emitted duty is the preset's, never the payload's.
func TestMaterializeIgnoresModelSuppliedParameters(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	doc, err := catalog.Materialize(actions.Command{
		CommandID: "cmd-3", EffectorRoute: "select_thermal_mode", IdempotencyKey: idemKey(),
		Payload: map[string]any{"mode": "bounded_cooling", "duty_permille": float64(999), "lease_ms": float64(99999), "target": "pump-01"},
	}, boot)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if doc["target"] != "fan-01" {
		t.Fatalf("target came from payload injection: %v", doc["target"])
	}
	params := doc["parameters"].(map[string]any)
	if params["duty_permille"].(float64) != 450 {
		t.Fatalf("duty came from payload injection: %v", params["duty_permille"])
	}
}

func TestMaterializeFailsClosed(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	cases := []struct {
		name string
		cmd  actions.Command
		boot string
	}{
		{"route not in catalog", actions.Command{CommandID: "c", EffectorRoute: "raise_manual_review", IdempotencyKey: idemKey(), Payload: map[string]any{"mode": "hold"}}, boot},
		{"selector not an allowed preset", actions.Command{CommandID: "c", EffectorRoute: "select_thermal_mode", IdempotencyKey: idemKey(), Payload: map[string]any{"mode": "turbo"}}, boot},
		{"missing selector field", actions.Command{CommandID: "c", EffectorRoute: "select_thermal_mode", IdempotencyKey: idemKey(), Payload: map[string]any{"entity_id": "zone-01"}}, boot},
		{"empty boot id", actions.Command{CommandID: "c", EffectorRoute: "set_indicator", IdempotencyKey: idemKey(), Payload: map[string]any{"state": "off"}}, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := catalog.Materialize(tc.cmd, tc.boot); err == nil {
				t.Fatalf("expected %s to fail closed", tc.name)
			}
		})
	}
}

// Defense in depth: even if a catalog preset were mis-authored above a hard
// bound, the materializer rejects it rather than emitting an over-bound command.
func TestMaterializeReEnforcesHardBounds(t *testing.T) {
	t.Parallel()
	overBound := `{
      "protocol_version": 1,
      "routes": {
        "select_thermal_mode": {
          "operation": "set_pwm_lease", "target": "fan-01", "selector_field": "mode",
          "expires_after_ms": 20000,
          "presets": {"bounded_cooling": {"duty_permille": 900, "lease_ms": 5000}},
          "bounds": {"duty_permille": {"max": 600}}
        }
      }
    }`
	catalog, err := actions.LoadCapabilityCatalog([]byte(overBound))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := catalog.Materialize(actions.Command{
		CommandID: "c", EffectorRoute: "select_thermal_mode", IdempotencyKey: idemKey(),
		Payload: map[string]any{"mode": "bounded_cooling"},
	}, boot); err == nil {
		t.Fatal("preset above hard max must be rejected at materialization")
	}
}

func TestLoadCapabilityCatalogRejectsInvalid(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no routes":            `{"protocol_version": 1, "routes": {}}`,
		"bad protocol":         `{"protocol_version": 0, "routes": {"r": {"operation": "o", "target": "t", "selector_field": "s", "expires_after_ms": 1, "presets": {"p": {}}}}}`,
		"unknown field":        `{"protocol_version": 1, "gremlin": true, "routes": {}}`,
		"bound without preset": `{"protocol_version": 1, "routes": {"r": {"operation": "o", "target": "t", "selector_field": "s", "expires_after_ms": 1, "presets": {"p": {}}, "bounds": {"x": {"max": 1}}}}}`,
	}
	for name, body := range cases {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := actions.LoadCapabilityCatalog([]byte(body)); err == nil {
				t.Fatalf("expected %s to be rejected", name)
			}
		})
	}
}
