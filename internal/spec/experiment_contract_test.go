package spec_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const zoneSchemasDigest = "sha256:eb765c6cd6ccfaf67c2be6ba2bae91694fb079020ddd065871ba1849e2f81a4d"

var experimentSpecs = []struct{ path, digest string }{
	{"../../examples/thermal-chamber/zone-thermal.situation.yaml", "sha256:d5907b52280fc34ec1d17c88e905c2ac79ad4358eb4bb2253e6f7efdf0f925fb"},
	{"../../examples/real-world-sensor/zone-thermal-sim.situation.yaml", "sha256:a6153efe15c9a2b5ea7706d8e8f62312263b8c994eb6839caeb45b84412ae171"},
	{"../../examples/real-world-sensor/zone-thermal-bench.situation.yaml", "sha256:b1b60be7ddae1ef91da2b43f52ee44263f6d2a367bfdd0605bdd1c28ad9c34cf"},
}

func TestExperimentSpecsCompileToTheirPinnedDigests(t *testing.T) {
	t.Parallel()
	for _, pinned := range experimentSpecs {
		t.Run(filepath.Base(pinned.path), func(t *testing.T) {
			t.Parallel()
			compiled, err := spec.CompileFile(t.Context(), pinned.path)
			if err != nil {
				t.Fatalf("%s no longer compiles; the real-world-sensor runbooks run it: %v", pinned.path, err)
			}
			if compiled.Digest != pinned.digest {
				t.Errorf("%s compiles to %s; update the real-world-sensor runbook and the gateway --device-policy-digest in the same change, then this pin (docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md)", pinned.path, compiled.Digest)
			}
		})
	}
}

func TestExperimentZoneEventSchemasAreUnchanged(t *testing.T) {
	t.Parallel()
	digest := zoneSchemaRegistryDigest(t)
	if digest != zoneSchemasDigest {
		t.Errorf("zone.* event schemas changed (%s); update the real-world-sensor DHT11 mapping and the Streams Simulator thermal domain first (docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md)", digest)
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
