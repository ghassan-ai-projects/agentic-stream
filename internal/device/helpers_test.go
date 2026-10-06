package device_test

import (
	"context"
	"time"

	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
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

// deviceControl is an admitted runtime owner with its device authority.
type deviceControl struct {
	authority *deviceauthority.Service
}

func newDeviceControl(t *testing.T) deviceControl {
	t.Helper()
	db, err := storage.Open(context.Background(), t.TempDir()+"/device.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(context.Background(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &runtimecontrol.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatal(err)
	}
	return deviceControl{authority: authority}
}
