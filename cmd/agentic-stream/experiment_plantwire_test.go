package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

type plantWireServe struct {
	dir, socket string
	stop        func()
}

func startPlantWireServe(t *testing.T, kindsDir string) plantWireServe {
	t.Helper()
	dir := privateSocketDir(t)
	site := filepath.Join(dir, "bench.site.json")
	if err := os.WriteFile(site, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	run := plantWireServe{dir: dir, socket: filepath.Join(dir, "host.sock")}
	address := freeLoopbackAddress(t)
	ctx, cancel := context.WithCancel(t.Context())
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", newMigratedDatabasePath(t), "--spec", testSpec, "--effect-profile", "emulator",
		"--plant-wire-socket", run.socket, "--plant-wire-kinds", kindsDir, "--plant-wire-site", site,
		"--plant-wire-epoch", "11", "--listen", address, "--poll-interval", "50ms"})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	waitReady(t, "http://"+address+"/health/live", done)
	run.stop = func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve returned %v after cancellation", err)
		}
	}
	t.Cleanup(run.stop)
	return run
}

func TestServeHostsPlantWireSessionsOneAfterAnother(t *testing.T) {
	t.Parallel()
	run := startPlantWireServe(t, plantwirefake.KindsDir(t, "."))
	for _, session := range []string{"s-1", "s-2"} {
		node := plantwirefake.Dial(t, "unix", run.socket, session)
		node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
		if state := node.Expect("STATE"); state["online"] != true || state["authority_epoch"] != json.Number("11") {
			t.Fatalf("%s: STATE = %v, want online at epoch 11", session, state)
		}
		node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": 1767225660000000000})
		node.Expect("HSYNC")
		node.Send(map[string]any{"type": "NDEATH", "lane": "control", "bd_seq": 0, "reason": "shutdown"})
		if !node.Closed() {
			t.Fatalf("%s: the host kept the session after the node's death", session)
		}
	}
}
