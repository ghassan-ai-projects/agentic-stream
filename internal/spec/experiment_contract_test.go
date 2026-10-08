package spec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// The real-world-sensor experiment depends on two spec surfaces it cannot see
// change from its own repository: the compiled digest of the thermal spec,
// which is the policy version every device command's policy digest is derived
// from (the bench gateway allow-lists that digest), and the zone.* event
// schemas its gateway and the Streams Simulator emit. Change a pin only
// together with the named consumer. See
// docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md.

const (
	zoneThermalSpec   = "../../docs/design/examples/zone-thermal.situation.yaml"
	zoneThermalDigest = "sha256:d5907b52280fc34ec1d17c88e905c2ac79ad4358eb4bb2253e6f7efdf0f925fb"
	zoneSchemasDigest = "sha256:eb765c6cd6ccfaf67c2be6ba2bae91694fb079020ddd065871ba1849e2f81a4d"
)

func TestExperimentThermalSpecCompilesToItsPinnedDigest(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(context.Background(), zoneThermalSpec)
	if err != nil {
		t.Fatalf("zone-thermal no longer compiles; the real-world-sensor runbook copies it: %v", err)
	}
	if compiled.Digest != zoneThermalDigest {
		t.Errorf("zone-thermal compiles to %s; update the real-world-sensor runbook and the gateway --device-policy-digest in the same change, then this pin", compiled.Digest)
	}
}

func TestExperimentZoneEventSchemasAreUnchanged(t *testing.T) {
	t.Parallel()
	digest := zoneSchemaRegistryDigest(t)
	if digest != zoneSchemasDigest {
		t.Errorf("zone.* event schemas changed (%s); update the real-world-sensor DHT11 mapping and the Streams Simulator thermal domain first", digest)
	}
}

func zoneSchemaRegistryDigest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("internal/domain/event_schema_data.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry map[string]any
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	zone := map[string]any{}
	for name, schema := range registry {
		if strings.HasPrefix(name, "zone.") {
			zone[name] = schema
		}
	}
	canonical, err := canonicaljson.Marshal(zone)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}
