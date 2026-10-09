package device_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func loadThermalCatalog(t *testing.T) *device.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json")
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

func firmwareDigest() string { return "sha256:" + strings.Repeat("c", 64) }

func newDeviceAuthority(t *testing.T) *deviceauthority.Service {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim runtime owner: %v", err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &runtimecontrol.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatalf("new device authority: %v", err)
	}
	return authority
}
