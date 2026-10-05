package device_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

func loadThermalCatalog(t *testing.T) *device.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	catalog, err := device.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func policyKey() string { return "sha256:" + strings.Repeat("b", 64) }
