package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
)

func TestServeRefusesToStartWhenTheCatalogDoesNotNameASpecIntent(t *testing.T) {
	t.Parallel()
	dir := privateSocketDir(t)
	database := filepath.Join(dir, "stream.db")
	seedMigratedDatabase(t, database)
	catalogPath, digest := catalogWithoutRoute(t, dir, "select_thermal_mode")
	serveTamozStandIn(t, filepath.Join(dir, "worker.sock"), 0)
	serveDeviceStandIn(t, filepath.Join(dir, "device.sock"), digest)
	address := freeLoopbackAddress(t)
	cmd := newServeCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--db", database, "--spec", tamozActiveSpec(t, dir, experimentSpecEdits(experimentOptions{})),
		"--live-socket", filepath.Join(dir, "telemetry.sock"), "--trace-format", "normalized",
		"--worker-socket", filepath.Join(dir, "worker.sock"), "--worker-name", "tamoz",
		"--effect-profile", "emulator", "--device-socket", filepath.Join(dir, "device.sock"),
		"--device-catalog", catalogPath, "--device-firmware-digest", standInFirmwareDigest,
		"--listen", address, "--poll-interval", "50ms",
	})
	err := cmd.ExecuteContext(t.Context())
	if err == nil || !strings.Contains(err.Error(), `route "select_thermal_mode" has no effector`) || !strings.Contains(err.Error(), "device catalog does not name it") {
		t.Fatalf("serve with an intent the catalog does not name = %v, want it refused at start naming the intent", err)
	}
	conn, dialErr := (&net.Dialer{}).DialContext(t.Context(), "tcp", address)
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatalf("dial after a refused start = %v, want connection refused: serve must not listen", dialErr)
	}
}

func catalogWithoutRoute(t *testing.T, dir, route string) (string, string) {
	t.Helper()
	data, err := os.ReadFile(experimentCatalog)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	routes, ok := document["routes"].(map[string]any)
	if !ok {
		t.Fatal("the experiment catalog has no routes object")
	}
	delete(routes, route)
	trimmed, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := device.LoadCapabilityCatalog(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "catalog-without-"+route+".json")
	if err := os.WriteFile(path, trimmed, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, digest
}
