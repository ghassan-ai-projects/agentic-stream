package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "agentic-stream version") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "contract: situation-runtime-contracts/v1") {
		t.Fatalf("missing contract version: %s", out)
	}
	if !strings.Contains(out, "protocol: agenticstream.runtime/v1") {
		t.Fatalf("missing protocol version: %s", out)
	}
}

func TestServeCommandRequiresContinuousSourcePair(t *testing.T) {
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", "runtime.db", "--spec", "spec.yaml"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--spec and --trace must be provided together") {
		t.Fatalf("expected paired source validation, got %v", err)
	}
}

func TestServeCommandRefusesPhysicalProfileWithJSONLSource(t *testing.T) {
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", "runtime.db", "--spec", "spec.yaml", "--trace", "trace.jsonl", "--effect-profile", "physical"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "physical effect profile cannot be combined with replay or shadow") {
		t.Fatalf("expected physical/JSONL isolation error, got %v", err)
	}
}

func TestServeSourceValidationAllowsLiveSocketWithWorker(t *testing.T) {
	if err := validateServeSources("spec.yaml", "", "/tmp/live.sock", "/tmp/worker.sock"); err != nil {
		t.Fatalf("live socket plus worker rejected: %v", err)
	}
}

func TestServeSourceValidationRejectsMixedSources(t *testing.T) {
	err := validateServeSources("spec.yaml", "trace.jsonl", "/tmp/live.sock", "")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mixed source rejection, got %v", err)
	}
}

func TestServeSourceValidationRequiresSpecForLiveSocket(t *testing.T) {
	err := validateServeSources("", "", "/tmp/live.sock", "")
	if err == nil || !strings.Contains(err.Error(), "--spec and --live-socket") {
		t.Fatalf("expected live source/spec pairing error, got %v", err)
	}
}

func TestLoopbackListenAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:8080": true,
		"[::1]:8080":     true,
		"0.0.0.0:8080":   false,
		"127.0.0.1":      false,
	} {
		if got := isLoopbackListenAddress(address); got != want {
			t.Errorf("isLoopbackListenAddress(%q) = %v, want %v", address, got, want)
		}
	}
}

func TestWorkerRuntimeFlagsMatchAcrossLiveCommands(t *testing.T) {
	runLive := newRunLiveCommand()
	serve := newServeCommand()
	flagNames := []string{
		"worker-socket", "model-endpoint", "model-name", "worker-name", "worker-ca",
		"worker-cert", "worker-key", "worker-server-name", "evidence-socket", "evidence-key",
	}
	for _, name := range flagNames {
		runLiveFlag := runLive.Flags().Lookup(name)
		serveFlag := serve.Flags().Lookup(name)
		if runLiveFlag == nil || serveFlag == nil {
			t.Fatalf("shared flag %q missing: run-live=%v serve=%v", name, runLiveFlag != nil, serveFlag != nil)
		}
		if runLiveFlag.DefValue != serveFlag.DefValue || runLiveFlag.Usage != serveFlag.Usage || runLiveFlag.Value.Type() != serveFlag.Value.Type() {
			t.Fatalf("shared flag %q diverged: run-live=(%q,%q,%q) serve=(%q,%q,%q)",
				name, runLiveFlag.DefValue, runLiveFlag.Usage, runLiveFlag.Value.Type(), serveFlag.DefValue, serveFlag.Usage, serveFlag.Value.Type())
		}
	}
}

func TestWorkerRuntimeErrorMonitorForwardsAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerErrors := make(chan error, 1)
	failures := make(chan error, 1)
	done := monitorWorkerRuntimeErrors(ctx, workerErrors, failures, cancel)
	want := errors.New("evidence server failed")
	workerErrors <- want

	select {
	case got := <-failures:
		if !errors.Is(got, want) {
			t.Fatalf("forwarded error = %v, want to wrap %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("worker runtime error was not forwarded")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("worker runtime error did not cancel the route")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker runtime error monitor did not stop")
	}
}
