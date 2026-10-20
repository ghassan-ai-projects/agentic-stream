package composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const thermalCatalogPath = "../../../contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json"

var standInFirmwareDigest = "sha256:" + strings.Repeat("c", 64)

type handshakeOnlyGateway struct {
	state     []byte
	handshake bool
}

func (g *handshakeOnlyGateway) Send(context.Context, []byte) error {
	return errors.New("the start check sends no command")
}

func (g *handshakeOnlyGateway) Receive(context.Context) ([]byte, error) {
	if g.handshake {
		return nil, errors.New("the gateway sends only its handshake")
	}
	g.handshake = true
	return g.state, nil
}

func (g *handshakeOnlyGateway) QueryState(context.Context) ([]byte, error) {
	return g.state, nil
}

func (g *handshakeOnlyGateway) Close() error {
	return nil
}

func TestWithADeviceTheStartCheckAcceptsOnlyTheRoutesItsCatalogNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		omit     []string
		fallback actionport.RoutedEffector
		want     string
	}{
		{"a catalog that names every intent starts under the simulated fallback", nil, device.NewSimulatedEffector(), ""},
		{"a catalog that names every intent starts under a fail-closed fallback", nil, device.NewFailClosedEffector(device.EffectProfileEmulator), ""},
		{"a simulated fallback does not cover a route the catalog omits", []string{"select_thermal_mode"}, device.NewSimulatedEffector(), `route "select_thermal_mode" has no effector: it is not the watch route and the device catalog does not name it`},
		{"a fail-closed fallback does not cover a route the catalog omits", []string{"set_indicator"}, device.NewFailClosedEffector(device.EffectProfilePhysical), `route "set_indicator" has no effector: it is not the watch route and the device catalog does not name it`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := spec.CompileFile(t.Context(), activeThermalSpecPath)
			if err != nil {
				t.Fatal(err)
			}
			db := storagetest.OpenTemp(t)
			pipeline, err := runtime.NewPipeline(t.Context(), runtime.PipelineConfig{
				DB: db, Spec: compiled, TenantID: "default", Clock: sources.Physical(),
				IDGenerator: sources.Deterministic(), Executor: fixture.New(), Effector: tt.fallback,
				GatewayEffector: openHandshakeGateway(t, db, thermalCatalogWithout(t, tt.omit...)),
			})
			if err == nil {
				t.Cleanup(func() { _ = pipeline.Close() })
			}
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("NewPipeline = %v, want %q", err, tt.want)
			}
		})
	}
}

func thermalCatalogWithout(t *testing.T, routes ...string) *device.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile(thermalCatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	named, _ := document["routes"].(map[string]any)
	for _, route := range routes {
		delete(named, route)
	}
	trimmed, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := device.LoadCapabilityCatalog(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func openHandshakeGateway(t *testing.T, db *storage.DB, catalog *device.CapabilityCatalog) *device.GatewayEffector {
	t.Helper()
	effector, closeEffector, err := device.NewGatewayEffector(t.Context(), device.GatewayEffectorConfig{
		Transport: &handshakeOnlyGateway{state: deviceStateFrame(t, catalog)}, Catalog: catalog,
		AllowedFirmwareDigests: []string{standInFirmwareDigest}, OwnerEpoch: "epoch-1", OwnerInstance: "instance-1",
		Authority: claimedDeviceAuthority(t, db),
	})
	if err != nil {
		t.Fatalf("open gateway effector: %v", err)
	}
	t.Cleanup(func() { _ = closeEffector() })
	return effector
}

func deviceStateFrame(t *testing.T, catalog *device.CapabilityCatalog) []byte {
	t.Helper()
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := canonicaljson.Marshal(map[string]any{
		"message_type": "state", "protocol_version": float64(1), "device_id": "thermal-01", "boot_id": "boot-A",
		"firmware_digest": standInFirmwareDigest, "capability_digest": digest, "safe_state": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func claimedDeviceAuthority(t *testing.T, db *storage.DB) *deviceauthority.Service {
	t.Helper()
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
