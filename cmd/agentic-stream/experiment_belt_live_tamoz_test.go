package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	liveModelVariable    = "CONVEYOR_LIVE_MODEL"
	liveTamozDirVariable = "CONVEYOR_TAMOZ_DIR"
	liveOutVariable      = "CONVEYOR_LIVE_OUT"
	liveRowsVariable     = "CONVEYOR_LIVE_ROWS"
	liveKeyVariable      = "ZAI_API_KEY"
	liveProvider         = "zai"
	liveModelID          = "glm-5.3-flash"
	liveModelCallCap     = 60
	liveWorkerStartup    = time.Minute
	liveWorkerShutdown   = 10 * time.Second
)

var errModelCallCap = errors.New("the run used more model calls than its cap")

const liveWorkerProfile = `profile:
  schema_version: 1
  profile_id: conveyor-live-worker
  profile_version: "1.0"
  canonical_root: %[1]s
roots:
  workspace: %[1]s
tools:
  allowed: [read_file, list_directory]
policy:
  allow_changes: false
  default_check_safety: read_only
  graph_version: "1"
  behavior_version: "1.0"
  tool_catalog_digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
model_roles:
  conveyor_reasoner:
    provider: %[2]s
    model: %[3]s
`

type liveModel struct {
	tamozDir, out string
	rows          []int
}

func liveModelUnderTest(t *testing.T) liveModel {
	t.Helper()
	if os.Getenv(liveModelVariable) != "1" {
		t.Skipf("%s is not 1: the live-model conveyor runs call a real model and run only when asked", liveModelVariable)
	}
	tamozDir := os.Getenv(liveTamozDirVariable)
	if !holdsTamozWorker(tamozDir) {
		t.Skipf("%s=%q holds no bin/tamoz-stream-worker", liveTamozDirVariable, tamozDir)
	}
	if os.Getenv(liveKeyVariable) == "" {
		t.Skipf("%s is not set: the Tamoz worker cannot call %s", liveKeyVariable, liveProvider)
	}
	return liveModel{tamozDir: tamozDir, out: cmp.Or(os.Getenv(liveOutVariable), t.TempDir()), rows: liveRows(t)}
}

func holdsTamozWorker(tamozDir string) bool {
	checkout, err := os.OpenRoot(cmp.Or(tamozDir, "."))
	if tamozDir == "" || err != nil {
		return false
	}
	defer func() { _ = checkout.Close() }()
	_, err = checkout.Stat(filepath.Join("bin", "tamoz-stream-worker"))
	return err == nil
}

func liveRows(t *testing.T) []int {
	t.Helper()
	var rows []int
	for _, field := range strings.Split(cmp.Or(os.Getenv(liveRowsVariable), "1"), ",") {
		row, err := strconv.Atoi(field)
		if err != nil || row < 1 || row > 10 {
			t.Fatalf("%s=%q: want seed row numbers 1 to 10", liveRowsVariable, os.Getenv(liveRowsVariable))
		}
		rows = append(rows, row)
	}
	return rows
}

func (l liveModel) runFolder(t *testing.T, name string) string {
	t.Helper()
	out := filepath.Join(l.out, name)
	if err := os.MkdirAll(l.out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatalf("%s: %v (a run keeps its own folder; move the earlier one away)", out, err)
	}
	return out
}

func (l liveModel) startTamoz(t *testing.T, out string) string {
	t.Helper()
	root := filepath.Join(out, "worker-root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(out, "worker-profile.yml")
	writePrivateFile(t, profile, fmt.Sprintf(liveWorkerProfile, root, liveProvider, liveModelID))
	socket := filepath.Join(privateSocketDir(t), "tamoz.sock")
	log, err := os.OpenFile(filepath.Join(out, "worker.log"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600) //nolint:gosec // the run folder is the test's own
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	rbenv, err := exec.LookPath("rbenv")
	if err != nil {
		t.Fatalf("the Tamoz worker runs under rbenv: %v", err)
	}
	lanes := fmt.Sprintf("fast=%[1]s,deep=%[1]s,batch=%[1]s", liveModelID)
	process := &exec.Cmd{Path: rbenv, Dir: l.tamozDir, Stdout: log, Stderr: log, Env: append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"),
		Args: []string{rbenv, "exec", "ruby", "bin/tamoz-stream-worker", "--profile", profile, "--database", filepath.Join(out, "tamoz.db"),
			"--tenant", "default", "--socket", socket, "--lane-config", lanes}}
	if err := process.Start(); err != nil {
		t.Fatalf("start the Tamoz worker: %v", err)
	}
	t.Cleanup(func() { stopTamoz(process) })
	awaitSocket(t, socket)
	return socket
}

func stopTamoz(process *exec.Cmd) {
	_ = process.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(liveWorkerShutdown):
		_ = process.Process.Kill()
		<-done
	}
}

func awaitSocket(t *testing.T, socket string) {
	t.Helper()
	bound := pollUntil(t, liveWorkerStartup, func() bool {
		info, err := os.Stat(socket)
		return err == nil && info.Mode()&os.ModeSocket != 0
	})
	if bound {
		return
	}
	t.Fatalf("the Tamoz worker did not bind %s within %s: see worker.log", socket, liveWorkerStartup)
}

type tamozMeter struct {
	forward                                    *campaign.Worker
	episodes, calls, inputTokens, outputTokens atomic.Uint64
	refused                                    atomic.Bool
}

func serveMeteredTamoz(t *testing.T, out, upstream string) (*tamozMeter, string) {
	t.Helper()
	conn, err := workerfake.DialEvidenceSocket(t.Context(), upstream)
	if err != nil {
		t.Fatalf("dial the Tamoz worker: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	meter := &tamozMeter{forward: &campaign.Worker{Recording: campaign.Record(filepath.Join(out, "responses.jsonl")),
		Upstream: runtimev1.NewEpisodeWorkerClient(conn), LogExchanges: true}}
	socket := filepath.Join(privateSocketDir(t), "worker.sock")
	listener, err := worker.ListenEvidenceSocket(socket)
	if err != nil {
		t.Fatalf("listen worker socket: %v", err)
	}
	served := meter.forward.Server(false)
	served.ExecuteFunc = meter.execute
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, served)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return meter, socket
}

func (m *tamozMeter) execute(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	if m.calls.Load() >= liveModelCallCap {
		m.refused.Store(true)
		return errModelCallCap
	}
	m.episodes.Add(1)
	return m.forward.Execute(ctx, req, func(event *runtimev1.EpisodeEvent) error {
		if completed := event.GetModelCompleted(); completed != nil {
			m.inputTokens.Add(completed.GetUsage().GetInputTokens())
			m.outputTokens.Add(completed.GetUsage().GetOutputTokens())
			if m.calls.Add(1) > liveModelCallCap {
				m.refused.Store(true)
				return errModelCallCap
			}
		}
		return emit(event)
	})
}

func (m *tamozMeter) report(t *testing.T, out string, wall time.Duration) {
	t.Helper()
	metered := map[string]any{"episodes": m.episodes.Load(), "model_calls": m.calls.Load(), "input_tokens": m.inputTokens.Load(),
		"output_tokens": m.outputTokens.Load(), "model_call_cap": liveModelCallCap, "aborted_at_cap": m.refused.Load(), "wall_s": wall.Seconds()}
	raw, err := json.MarshalIndent(metered, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writePrivateFile(t, filepath.Join(out, "metered.json"), string(raw)+"\n")
	t.Logf("Tamoz: %s", raw)
	if m.refused.Load() {
		t.Errorf("aborted: the run reached its cap of %d model calls; later episodes were refused", liveModelCallCap)
	}
}
