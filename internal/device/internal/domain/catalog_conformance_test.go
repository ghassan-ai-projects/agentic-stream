package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestThermalCapabilityCatalogDigestIsPinned(t *testing.T) {
	t.Parallel()
	data := thermalCatalogBytes(t)
	catalog, err := domain.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load canonical catalog: %v", err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest canonical catalog: %v", err)
	}
	if digest != thermalCapabilityCatalogHash {
		t.Fatalf("canonical catalog digest = %q, want %q; the bench firmware reports this digest, so changing it needs a firmware rebuild", digest, thermalCapabilityCatalogHash)
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
