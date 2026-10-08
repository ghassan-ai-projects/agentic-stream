package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testSpec  = "../../docs/design/examples/predictive-maintenance.situation.yaml"
	testTrace = "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

func TestRunLiveProcessesTraceEndToEnd(t *testing.T) {
	disableTelemetryExport(t)
	cmd := newRunLiveCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "runtime.db"), "--spec", testSpec, "--trace", testTrace})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("run-live: %v", err)
	}
	if !strings.Contains(out.String(), "events_ingested=") || strings.Contains(out.String(), "events_ingested=0 ") {
		t.Fatalf("run-live reported no ingestion: %q", out.String())
	}
}

func TestRunLiveRejectsInvalidInputsBeforeOpeningState(t *testing.T) {
	disableTelemetryExport(t)
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing trace", args: []string{"--db", dbPath, "--spec", testSpec}, want: "--spec, --trace, and --db are required"},
		{name: "unknown trace format", args: []string{"--db", dbPath, "--spec", testSpec, "--trace", testTrace, "--trace-format", "csv"}, want: `unsupported --trace-format "csv"`},
		{name: "physical profile on a replay source", args: []string{"--db", dbPath, "--spec", testSpec, "--trace", testTrace, "--effect-profile", "physical"}, want: "validate effect profile"},
		{name: "missing spec file", args: []string{"--db", dbPath, "--spec", "missing.yaml", "--trace", testTrace}, want: "compile spec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRunLiveCommand()
			cmd.SetArgs(tt.args)
			err := cmd.ExecuteContext(t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestServeFlagsValidate(t *testing.T) {
	valid := func() serveFlags {
		return serveFlags{
			liveFlags:     liveFlags{dbPath: "runtime.db", traceFormat: "normalized", effectProfile: "simulated"},
			listenAddress: "127.0.0.1:8080", pollInterval: time.Second,
		}
	}
	tests := []struct {
		name   string
		mutate func(*serveFlags)
		token  string
		want   string
	}{
		{name: "valid", mutate: func(*serveFlags) {}, token: "secret"},
		{name: "missing db", mutate: func(f *serveFlags) { f.dbPath = "" }, token: "secret", want: "--db is required"},
		{name: "live socket needs normalized", mutate: func(f *serveFlags) {
			f.specPath, f.liveSocket, f.traceFormat = "spec.yaml", "/tmp/live.sock", "simulator"
		}, token: "secret", want: "--live-socket requires --trace-format normalized"},
		{name: "worker flags", mutate: func(f *serveFlags) { f.worker.EvidenceKey = "00" }, token: "secret", want: "worker runtime config"},
		{name: "poll interval", mutate: func(f *serveFlags) { f.pollInterval = 0 }, token: "secret", want: "--poll-interval must be positive"},
		{name: "subscriber token", mutate: func(*serveFlags) {}, want: "AGENTIC_STREAM_SUBSCRIBER_TOKEN is required"},
		{name: "non-loopback listen", mutate: func(f *serveFlags) { f.listenAddress = "0.0.0.0:8080" }, token: "secret", want: "non-loopback --listen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN", tt.token)
			flags := valid()
			tt.mutate(&flags)
			token, err := flags.validate()
			if tt.want == "" {
				if err != nil || token != tt.token {
					t.Fatalf("validate = %q, %v", token, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestServeRunsContinuousPipelineUntilCanceled(t *testing.T) {
	disableTelemetryExport(t)
	t.Setenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN", "subscriber-secret")
	address := freeLoopbackAddress(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "runtime.db"), "--spec", testSpec, "--trace", testTrace,
		"--listen", address, "--poll-interval", "10ms"})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	waitReady(t, "http://"+address+"/health/live", done)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v after cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}

func TestRunTraceRejectsUnknownFormat(t *testing.T) {
	if _, err := runTrace(t.Context(), nil, "csv", "trace.csv"); err == nil || !strings.Contains(err.Error(), "unsupported --trace-format") {
		t.Fatalf("runTrace error = %v", err)
	}
}

func TestCleanupsRunInReverseOrder(t *testing.T) {
	var order []int
	var cleanup cleanups
	for i := range 3 {
		cleanup.add(func() { order = append(order, i) })
	}
	cleanup.run()
	if len(order) != 3 || order[0] != 2 || order[2] != 0 {
		t.Fatalf("cleanup order = %v, want [2 1 0]", order)
	}
}

func disableTelemetryExport(t *testing.T) {
	t.Helper()
	for _, key := range []string{"AGENTIC_STREAM_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		t.Setenv(key, "")
	}
}

var claimedLoopbackAddresses sync.Map

const loopbackPickAttempts = 50

func freeLoopbackAddress(t *testing.T) string {
	t.Helper()
	address, err := claimUnclaimedAddress(func() (string, error) { return pickLoopbackAddress(t.Context()) })
	if err != nil {
		t.Fatal(err)
	}
	return address
}

func claimUnclaimedAddress(pick func() (string, error)) (string, error) {
	for range loopbackPickAttempts {
		address, err := pick()
		if err != nil {
			return "", err
		}
		if _, taken := claimedLoopbackAddresses.LoadOrStore(address, struct{}{}); !taken {
			return address, nil
		}
	}
	return "", fmt.Errorf("no unclaimed loopback port after %d picks", loopbackPickAttempts)
}

func pickLoopbackAddress(ctx context.Context) (string, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve loopback port: %w", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", fmt.Errorf("release loopback port: %w", err)
	}
	return address, nil
}

func TestClaimedLoopbackAddressIsNeverHandedOutAgain(t *testing.T) {
	t.Parallel()
	offered := []string{"127.0.0.1:61001", "127.0.0.1:61001", "127.0.0.1:61002"}
	pick := func() (string, error) {
		next := offered[0]
		offered = offered[1:]
		return next, nil
	}
	first, err := claimUnclaimedAddress(func() (string, error) { return "127.0.0.1:61001", nil })
	if err != nil || first != "127.0.0.1:61001" {
		t.Fatalf("first claim = %q, %v", first, err)
	}
	second, err := claimUnclaimedAddress(pick)
	if err != nil || second != "127.0.0.1:61002" {
		t.Fatalf("second claim = %q, %v; want the next unclaimed port", second, err)
	}
}

func TestClaimGivesUpWhenEveryPickIsTaken(t *testing.T) {
	t.Parallel()
	taken := func() (string, error) { return "127.0.0.1:61003", nil }
	if _, err := claimUnclaimedAddress(taken); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if address, err := claimUnclaimedAddress(taken); err == nil {
		t.Fatalf("claimed %s although every pick was taken", address)
	}
}

const readyTimeout = time.Minute

func waitReady(t *testing.T, url string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("serve exited before becoming live: %v", err)
		default:
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("serve did not become live")
}
