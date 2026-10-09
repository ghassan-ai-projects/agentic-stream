package domain_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

const (
	bootID                       = "boot-A"
	thermalCapabilityCatalogPath = "../../../contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json"
	thermalCapabilityCatalogHash = "sha256:0d61225286c628cfba8cbf7aea514e1fdc95918b514b4b810516dbe0fc44fc76"
)

func thermalCatalogBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(thermalCapabilityCatalogPath)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	return data
}

func loadThermalCatalog(t *testing.T) *domain.CapabilityCatalog {
	t.Helper()
	catalog, err := domain.LoadCapabilityCatalog(thermalCatalogBytes(t))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func policyKey() string { return "sha256:" + strings.Repeat("b", 64) }

func request(route, target string, payload map[string]any) actionport.Command {
	return actionport.Command{
		CommandID: "cmd-1", EffectorRoute: route, NormalizedTarget: target,
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: payload,
	}
}

func documentOf(command domain.Command, err error) (map[string]any, error) {
	return command.Document(), err
}

func assertRefusal(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}
