package plantwire_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestHostServesANodeUntilItStops(t *testing.T) {
	t.Parallel()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, err := plantwire.New(plantwire.Config{Listener: listener, KindsDir: plantwirefake.KindsDir(t, "."),
		Settings: plantwire.Settings{Environment: plantwire.EnvironmentSimulated}})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx, discard) }()
	node := plantwirefake.Dial(t, "tcp", listener.Addr().String(), "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
	if state := node.Expect("STATE"); state["online"] != true {
		t.Fatalf("STATE = %v, want the session accepted", state)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil once stopped", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return once stopped")
	}
}

func TestNewAndCheckSettingsRefuseAnUnsafeHost(t *testing.T) {
	t.Parallel()
	mixed := plantwire.Settings{Environment: plantwire.EnvironmentMixed}
	if err := plantwire.CheckSettings(mixed); !errors.Is(err, plantwire.ErrHardwareWithoutConsent) {
		t.Errorf("CheckSettings(a mixed host without consent) = %v, want ErrHardwareWithoutConsent", err)
	}
	if _, err := plantwire.New(plantwire.Config{Settings: mixed}); !errors.Is(err, plantwire.ErrNoListener) {
		t.Errorf("New(no listener) = %v, want ErrNoListener", err)
	}
}

func TestCloseEndsRunWithoutAnError(t *testing.T) {
	t.Parallel()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, err := plantwire.New(plantwire.Config{Listener: listener, KindsDir: plantwirefake.KindsDir(t, "."),
		Settings: plantwire.Settings{Environment: plantwire.EnvironmentSimulated}})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(t.Context(), discard) }()
	if err := host.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run = %v after Close, want nil", err)
	}
}

func discard(context.Context, contractsv1.Envelope) error { return nil }

func TestTheHostIsTheEffectorOfItsCatalogsRoutes(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "real-world-sensor", "testdata", "plant-wire", "thermal-bench.catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := device.LoadCapabilityCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, err := plantwire.New(plantwire.Config{Listener: listener, KindsDir: plantwirefake.KindsDir(t, "."), Catalog: catalog,
		Settings: plantwire.Settings{Environment: plantwire.EnvironmentSimulated}})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })
	if !host.OwnsRoute("select_thermal_mode") || host.OwnsRoute(actionport.WatchInstallRoute) {
		t.Fatal("the host must own exactly its catalog's routes")
	}
	command := actionport.Command{CommandID: "c-1", EffectorRoute: "select_thermal_mode", NormalizedTarget: "fan-01", Payload: map[string]any{"mode": "hold"}}
	if _, err := host.Dispatch(t.Context(), command); !errors.Is(err, plantwire.ErrAuthorizationRequired) {
		t.Errorf("Dispatch = %v, want ErrAuthorizationRequired", err)
	}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	if _, err := host.DispatchAuthorized(t.Context(), command, allow); !errors.Is(err, plantwire.ErrDeviceNotInSite) {
		t.Errorf("DispatchAuthorized without a site file = %v, want ErrDeviceNotInSite", err)
	}
	if status, _, err := host.VerifyDeviceCommand(t.Context(), command); status != "" || err != nil {
		t.Errorf("VerifyDeviceCommand = %q, %v; want no final status yet", status, err)
	}
}
