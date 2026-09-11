package actions_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const thermalCapabilityCatalogDigest = "sha256:0d61225286c628cfba8cbf7aea514e1fdc95918b514b4b810516dbe0fc44fc76"

func TestThermalCapabilityCatalogDigest(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read canonical catalog: %v", err)
	}
	catalog, err := actions.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load canonical catalog: %v", err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest canonical catalog: %v", err)
	}
	if digest != thermalCapabilityCatalogDigest {
		t.Fatalf("canonical catalog digest = %q, want %q", digest, thermalCapabilityCatalogDigest)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode canonical catalog document: %v", err)
	}
	rawDigest, err := canonicaljson.Digest(canonicaljson.DomainCapabilityCatalog, document)
	if err != nil {
		t.Fatalf("digest canonical catalog document: %v", err)
	}
	if rawDigest != digest {
		t.Fatalf("catalog loader digest = %q, canonical document digest = %q", digest, rawDigest)
	}
}
