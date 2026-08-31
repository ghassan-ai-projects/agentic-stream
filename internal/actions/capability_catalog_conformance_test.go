package actions_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const thermalCapabilityCatalogDigest = "sha256:177552ccdaa8d71ac3eb1e27a1a60e3433e1e2792bad2dc16ea95acb2d735eaf"

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
