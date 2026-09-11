package actions_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const physicalCapabilityDigest = "sha256:0d61225286c628cfba8cbf7aea514e1fdc95918b514b4b810516dbe0fc44fc76"

func physicalSensorRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("REAL_WORLD_SENSOR_ROOT"); root != "" {
		return root
	}
	return filepath.Join("..", "..", "..", "agent-research-lab", "real-world-sensor")
}

func TestPhysicalArduinoCatalogMaterializesAndValidatesLEDAndFanCommands(t *testing.T) {
	data, err := os.ReadFile("../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read canonical physical catalog: %v", err)
	}
	catalog, err := actions.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load physical catalog: %v", err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest physical catalog: %v", err)
	}
	if digest != physicalCapabilityDigest {
		t.Fatalf("physical catalog digest = %s, want %s", digest, physicalCapabilityDigest)
	}

	// The physical-sensor repository is an optional sibling checkout in local
	// development. When present, verify that its consumer copy remains bound to
	// this repository's canonical catalog; when absent, the standalone contract
	// and materialization checks above still run in CI.
	externalPath := filepath.Join(physicalSensorRoot(t), "assessment", "arduino-mega-l293d-fan-led-capability-catalog.json")
	externalData, err := os.ReadFile(externalPath)
	if err != nil {
		if os.Getenv("REAL_WORLD_SENSOR_ROOT") != "" || !os.IsNotExist(err) {
			t.Fatalf("read physical catalog copy: %v", err)
		}
		t.Logf("optional cross-repository physical catalog is not checked out: %s", externalPath)
	} else {
		externalCatalog, err := actions.LoadCapabilityCatalog(externalData)
		if err != nil {
			t.Fatalf("load physical catalog copy: %v", err)
		}
		externalDigest, err := externalCatalog.Digest()
		if err != nil {
			t.Fatalf("digest physical catalog copy: %v", err)
		}
		if externalDigest != digest {
			t.Fatalf("physical catalog copy digest = %s, want canonical digest %s", externalDigest, digest)
		}
	}

	command, err := catalog.Materialize(actions.Command{
		CommandID:        "cross-repo-led",
		EffectorRoute:    "set_indicator",
		NormalizedTarget: "zone-01",
		IdempotencyKey:   "sha256:" + strings.Repeat("a", 64),
		PolicyDigest:     "sha256:" + strings.Repeat("b", 64),
		Payload:          map[string]any{"entity_id": "zone-01", "state": "alert"},
	}, "boot-cross")
	if err != nil {
		t.Fatalf("materialize physical LED command: %v", err)
	}
	if command["target"] != "led-01" || command["operation"] != "set_led" {
		t.Fatalf("materialized target/operation = %v/%v", command["target"], command["operation"])
	}
	if got, want := command["parameters"], map[string]any{"brightness_permille": float64(1000), "pattern": "solid"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("materialized physical parameters = %#v, want %#v", got, want)
	}
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, command); err != nil {
		t.Fatalf("materialized physical command must validate: %v", err)
	}

	safeStop, err := catalog.MaterializeSafeStop("led-01", "boot-cross")
	if err != nil {
		t.Fatalf("materialize physical safe stop: %v", err)
	}
	if safeStop["policy_digest"] != physicalCapabilityDigest || safeStop["operation"] != "safe_stop" {
		t.Fatalf("safe stop authority = %v/%v", safeStop["policy_digest"], safeStop["operation"])
	}
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, safeStop); err != nil {
		t.Fatalf("materialized physical safe stop must validate: %v", err)
	}

	fanCommand, err := catalog.Materialize(actions.Command{
		CommandID:        "cross-repo-fan",
		EffectorRoute:    "select_thermal_mode",
		NormalizedTarget: "zone-01",
		IdempotencyKey:   "sha256:" + strings.Repeat("d", 64),
		PolicyDigest:     "sha256:" + strings.Repeat("e", 64),
		Payload:          map[string]any{"entity_id": "zone-01", "mode": "bounded_cooling"},
	}, "boot-cross")
	if err != nil {
		t.Fatalf("materialize physical fan command: %v", err)
	}
	if fanCommand["target"] != "fan-01" || fanCommand["operation"] != "set_pwm_lease" {
		t.Fatalf("materialized fan target/operation = %v/%v", fanCommand["target"], fanCommand["operation"])
	}
	if got, want := fanCommand["parameters"], map[string]any{"duty_permille": float64(450), "lease_ms": float64(5000)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("materialized physical fan parameters = %#v, want %#v", got, want)
	}
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, fanCommand); err != nil {
		t.Fatalf("materialized physical fan command must validate: %v", err)
	}

	fanSafeStop, err := catalog.MaterializeSafeStop("fan-01", "boot-cross")
	if err != nil {
		t.Fatalf("materialize physical fan safe stop: %v", err)
	}
	if fanSafeStop["policy_digest"] != physicalCapabilityDigest || fanSafeStop["operation"] != "safe_stop" {
		t.Fatalf("fan safe stop authority = %v/%v", fanSafeStop["policy_digest"], fanSafeStop["operation"])
	}
	if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, fanSafeStop); err != nil {
		t.Fatalf("materialized physical fan safe stop must validate: %v", err)
	}
}
