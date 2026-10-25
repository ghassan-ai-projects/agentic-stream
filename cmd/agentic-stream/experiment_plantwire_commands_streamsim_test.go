package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

const benchPolicyDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"

func TestThePlantWireEffectorCommandsAPacedStreamsimSession(t *testing.T) {
	t.Parallel()
	binary, repository := streamsimNode(t)
	kinds := filepath.Join(repository, "examples", "devices", "thermal-board")
	host, socket := runCommandingHost(t, kinds)
	startPacedSession(t, binary, repository, kinds, socket)
	commands := []actionport.Command{benchCommand("c-led", "set_indicator", "state", "alert"), benchCommand("c-fan", "select_thermal_mode", "mode", "bounded_cooling")}
	for _, command := range commands {
		effect, err := dispatchOnceBorn(t, host, command)
		if err != nil || !effect.VerificationPending {
			t.Fatalf("%s: DispatchAuthorized = %v, %v; want accepted and succeeded, pending verification", command.CommandID, effect, err)
		}
		if status, evidence, err := host.VerifyDeviceCommand(t.Context(), command); status != actionport.CommandSucceeded || err != nil {
			t.Fatalf("%s: VerifyDeviceCommand = %q, %v, %v; want the twin's readings to verify it", command.CommandID, status, evidence, err)
		}
	}
	foreign := benchCommand("c-foreign", "set_indicator", "state", "watch")
	foreign.PolicyDigest = idempotencyKeyOf("another policy")
	if _, err := dispatchOnceBorn(t, host, foreign); !errors.Is(err, plantwire.ErrCommandRefused) || !strings.Contains(err.Error(), "policy_not_allowed") {
		t.Fatalf("a command under a policy the session does not allow = %v, want refused policy_not_allowed", err)
	}
}

func runCommandingHost(t *testing.T, kinds string) (*plantwire.Host, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(benchPlantWire, "thermal-bench.catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := device.LoadCapabilityCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(privateSocketDir(t), "host.sock")
	listener, err := worker.ListenEvidenceSocket(socket)
	if err != nil {
		t.Fatal(err)
	}
	host, err := plantwire.New(plantwire.Config{Listener: listener, Settings: plantwire.Settings{Environment: plantwire.EnvironmentSimulated},
		KindsDir: kinds, SitePath: filepath.Join(benchPlantWire, "thermal-bench.site.json"), Catalog: catalog,
		Authority: testDeviceAuthority(t), OwnerEpoch: testOwnerEpoch})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx, func(context.Context, contractsv1.Envelope) error { return nil }) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return host, socket
}

func startPacedSession(t *testing.T, binary, repository, kinds, socket string) {
	t.Helper()
	startPlannedSession(t, binary, repository, kinds, map[string]any{"schema_version": "0.1", "domain": "arduino-thermal",
		"host_socket": socket, "pace": "wall", "policies": []string{benchPolicyDigest}})
}

func startPlannedSession(t *testing.T, binary, repository, kinds string, sessionPlan map[string]any) {
	t.Helper()
	dir := t.TempDir()
	copyFile(t, filepath.Join(kinds, "thermal-board.domain.json"), dir)
	plan, err := json.Marshal(sessionPlan)
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(t.TempDir(), "session.plan.json")
	if err := os.WriteFile(planPath, plan, 0o600); err != nil {
		t.Fatal(err)
	}
	node := exec.Cmd{Path: binary, Args: []string{binary, "plantwire", "connect", "--plan", planPath, "--domains-dir", dir,
		"--adapters-dir", filepath.Join(repository, "adapters"), "--kinds-dir", kinds, "--out", filepath.Join(dir, "run")}}
	var output bytes.Buffer
	node.Stdout, node.Stderr = &output, &output
	if err := node.Start(); err != nil {
		t.Fatalf("start the paced session: %v", err)
	}
	t.Cleanup(func() {
		_ = node.Process.Kill()
		_ = node.Wait()
		if t.Failed() {
			t.Logf("the session's output:\n%s", output.String())
		}
	})
}

func benchCommand(id, route, selector, preset string) actionport.Command {
	return actionport.Command{CommandID: id, IntentID: "intent-" + id, EffectorRoute: route, NormalizedTarget: "zone-01",
		IdempotencyKey: idempotencyKeyOf(id), PolicyDigest: benchPolicyDigest, Payload: map[string]any{selector: preset}}
}

func dispatchOnceBorn(t *testing.T, host *plantwire.Host, command actionport.Command) (actionport.Effect, error) {
	t.Helper()
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	deadline := time.After(30 * time.Second)
	for {
		effect, err := host.DispatchAuthorized(t.Context(), command, allow)
		if !errors.Is(err, plantwire.ErrNoSession) && !errors.Is(err, plantwire.ErrDeviceNotBorn) {
			return effect, err
		}
		select {
		case <-deadline:
			t.Fatalf("%s: the device was never born: %v", command.CommandID, err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func idempotencyKeyOf(id string) string {
	sum := sha256.Sum256([]byte(id))
	return kernel.EncodeDigest(sum[:])
}

const testOwnerEpoch = "epoch-1"

func testDeviceAuthority(t *testing.T) *deviceauthority.Service {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Hour}
	if err := owner.Claim(t.Context(), testOwnerEpoch); err != nil {
		t.Fatal(err)
	}
	authority, err := newDeviceAuthority(db, owner, &runtimecontrol.EpochControl{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
