package domain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestPhysicalSensorRepositoryCopyOfTheCatalogMatchesTheCanonicalDigest(t *testing.T) {
	t.Parallel()
	root := os.Getenv("REAL_WORLD_SENSOR_ROOT")
	if root == "" {
		t.Skip("optional cross-repository check: REAL_WORLD_SENSOR_ROOT is not set")
	}
	data, err := os.ReadFile(filepath.Join(root, "assessment", "arduino-mega-l293d-fan-led-capability-catalog.json")) //nolint:gosec // root is the developer's own checkout named by REAL_WORLD_SENSOR_ROOT
	if err != nil {
		t.Fatalf("read physical catalog copy: %v", err)
	}
	catalog, err := domain.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load physical catalog copy: %v", err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest physical catalog copy: %v", err)
	}
	if digest != thermalCapabilityCatalogHash {
		t.Fatalf("physical catalog copy digest = %s, want canonical digest %s", digest, thermalCapabilityCatalogHash)
	}
}
