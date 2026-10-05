package app_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func loadThermalCatalog(t *testing.T) *domain.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../../../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	catalog, err := domain.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func policyKey() string { return "sha256:" + strings.Repeat("b", 64) }

func goldenDeviceState() map[string]any {
	return map[string]any{
		"message_type":      "state",
		"protocol_version":  1,
		"device_id":         "thermal-01",
		"boot_id":           "boot-A",
		"firmware_digest":   "sha256:" + strings.Repeat("c", 64),
		"capability_digest": "sha256:" + strings.Repeat("d", 64),
		"safe_state":        true,
	}
}
