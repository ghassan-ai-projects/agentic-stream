package transport

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const evidenceTestKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func probeNativeConstructor(t *testing.T, inspect func(nativeexecutor.Config)) NativeConstructor {
	t.Helper()
	return func(cfg nativeexecutor.Config) (*nativeexecutor.Executor, error) {
		inspect(cfg)
		tools := cfg.ToolFactory(&episodes.Request{TenantID: "tenant", EntityID: "motor"})
		if len(tools) != 2 {
			t.Errorf("tool count=%d, want the two evidence tool names scoped to the request", len(tools))
		}
		return nil, errNativeProbe
	}
}

var errNativeProbe = errors.New("constructor failure")

func TestNativeBackendUsesTheDeterministicProviderWithoutAModelEndpoint(t *testing.T) {
	t.Parallel()
	backend := NewWorkerBackend(WorkerRuntimeConfig{ModelName: "model"}, probeNativeConstructor(t, func(cfg nativeexecutor.Config) {
		if _, ok := cfg.Provider.(*nativeexecutor.DeterministicProvider); !ok {
			t.Errorf("provider=%T, want the deterministic provider", cfg.Provider)
		}
	}))
	if _, err := backend.NativeExecutor(); !errors.Is(err, errNativeProbe) {
		t.Fatalf("native error=%v, want the constructor failure", err)
	}
}

func TestNativeBackendUsesTheConfiguredModelEndpointWithTheKeyFromTheEnvironment(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_MODEL_API_KEY", "test-key")
	backend := NewWorkerBackend(WorkerRuntimeConfig{ModelEndpoint: "http://model.invalid", ModelName: "model"}, probeNativeConstructor(t, func(cfg nativeexecutor.Config) {
		p, ok := cfg.Provider.(*nativeexecutor.OpenAICompatibleProvider)
		if !ok || p.Endpoint != "http://model.invalid" || p.Model != "model" || p.APIKey != "test-key" {
			t.Errorf("provider=%+v", cfg.Provider)
		}
	}))
	if _, err := backend.NativeExecutor(); !errors.Is(err, errNativeProbe) {
		t.Fatalf("native error=%v, want the constructor failure", err)
	}
}

func TestBackendServesEvidenceThenConnectsTheWorkerAndReleasesBoth(t *testing.T) {
	t.Parallel()
	dir := workerfake.SocketDir(t)
	cfg := WorkerRuntimeConfig{DB: storagetest.OpenTemp(t), RuntimeEpoch: "epoch", WorkerSocket: filepath.Join(dir, "worker.sock"), WorkerName: "worker", EvidenceSocket: filepath.Join(dir, "evidence.sock"), EvidenceKey: evidenceTestKey}
	cfg.Ledger = testEvidenceLedger(t, cfg.DB)
	backend := NewWorkerBackend(cfg, nativeexecutor.New)
	t.Cleanup(func() { _ = backend.Close() })

	secret, err := backend.StartEvidence()
	if err != nil || len(secret) != 32 {
		t.Fatalf("evidence start: secret length=%d error=%v", len(secret), err)
	}
	executor, err := backend.ConnectWorker(t.Context(), secret)
	if err != nil || executor == nil || backend.workerConn == nil {
		t.Fatalf("worker connect: executor=%T error=%v", executor, err)
	}
	if backend.Errors() == nil {
		t.Fatal("missing async errors")
	}
	if err := backend.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close=%v", err)
	}
	if backend.workerConn != nil || backend.evidenceGRPC != nil || backend.evidenceListener != nil {
		t.Fatal("resources retained after close")
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("second close=%v", err)
	}
}

func TestConnectWorkerWithoutEvidenceNeedsNoEvidenceServer(t *testing.T) {
	t.Parallel()
	cfg := WorkerRuntimeConfig{DB: storagetest.OpenTemp(t), WorkerSocket: filepath.Join(workerfake.SocketDir(t), "worker.sock"), WorkerName: "worker"}
	backend := NewWorkerBackend(cfg, nativeexecutor.New)
	if _, err := backend.ConnectWorker(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if backend.evidenceGRPC != nil || backend.evidenceListener != nil {
		t.Fatal("an evidence server ran without an evidence socket")
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectWorkerReachesTheWorkerListeningOnItsSocket(t *testing.T) {
	t.Parallel()
	path := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	serveWorker(t, path, &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "1.0.0"})
	backend := NewWorkerBackend(WorkerRuntimeConfig{WorkerSocket: path, WorkerName: "tamoz"}, nativeexecutor.New)
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.ConnectWorker(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	response, err := runtimev1.NewEpisodeWorkerClient(backend.workerConn).Handshake(t.Context(), &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion, WorkerId: "tamoz", RuntimeInstanceId: "runtime", NonInteractive: true,
	})
	if err != nil || response.GetWorkerName() != "tamoz" {
		t.Fatalf("handshake over the dialed socket = %v, %v; want the worker's answer", response, err)
	}
}

func serveWorker(t *testing.T, path string, server *workerfake.Server) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatalf("listen worker socket: %v", err)
	}
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
}

func TestBackendSetupFailures(t *testing.T) {
	t.Parallel()
	occupiedPath := filepath.Join(workerfake.SocketDir(t), "evidence.sock")
	if err := os.WriteFile(occupiedPath, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		cfg  WorkerRuntimeConfig
		want string
	}{
		{"invalid key", WorkerRuntimeConfig{EvidenceKey: "zz"}, "--evidence-key"},
		{"missing ledger", WorkerRuntimeConfig{EvidenceKey: evidenceTestKey}, "evidence ledger is required"},
		{"occupied socket path", WorkerRuntimeConfig{EvidenceKey: evidenceTestKey, Ledger: testEvidenceLedger(t, storagetest.OpenTemp(t)), EvidenceSocket: occupiedPath}, "listen evidence socket: refusing unsafe existing socket path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backend := NewWorkerBackend(tt.cfg, nativeexecutor.New)
			t.Cleanup(func() {
				if err := backend.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err := backend.StartEvidence(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("evidence error=%v want %s", err, tt.want)
			}
		})
	}
	if content, err := os.ReadFile(occupiedPath); err != nil || string(content) != "existing file" {
		t.Fatalf("occupied path was changed: content=%q error=%v", content, err)
	}
}

func TestConnectWorkerNamesTheStepThatRefusedTheConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  WorkerRuntimeConfig
		want string
	}{
		{"unreadable TLS material", WorkerRuntimeConfig{WorkerCA: "/missing/ca"}, "read worker CA"},
		{"a socket path the worker protocol forbids", WorkerRuntimeConfig{WorkerSocket: "relative/worker.sock"}, "dial episode worker socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewWorkerBackend(tt.cfg, nativeexecutor.New).ConnectWorker(t.Context(), nil); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("connect error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestAnAbsentBackendHasNoErrorsAndClosesCleanly(t *testing.T) {
	t.Parallel()
	var absent *WorkerBackend
	if absent.Errors() != nil || absent.Close() != nil {
		t.Fatal("nil backend lifecycle changed")
	}
}

func testEvidenceLedger(t *testing.T, db *storage.DB) *evidence.Service {
	t.Helper()
	service, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{DB: db, LeaseOwner: "fixture", RuntimeEpoch: "epoch", OwnerCheck: func(context.Context, *sql.Tx, string) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestEvidenceCapabilityFactoryUsesTheEvidenceReadBudget(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	factory := evidenceCapabilityFactory(nil, "epoch-1", now)
	if factory.MaxRows != evidence.DefaultReadMaxRows || factory.MaxBytes != evidence.DefaultReadMaxBytes {
		t.Errorf("budget = %d rows, %d bytes", factory.MaxRows, factory.MaxBytes)
	}
	if !factory.From.Equal(now.Add(-evidence.DefaultReadWindow)) || !factory.Until.Equal(now.Add(evidence.DefaultReadWindow)) {
		t.Errorf("window = %v..%v", factory.From, factory.Until)
	}
}

func TestWorkerOptionsProjectEveryConfiguredField(t *testing.T) {
	t.Parallel()
	cfg := WorkerRuntimeConfig{RuntimeEpoch: "epoch", WorkerSocket: "w.sock", WorkerName: "worker", WorkerCA: "ca", WorkerCert: "cert", WorkerKey: "key", WorkerServerName: "server", EvidenceSocket: "e.sock", EvidenceKey: "k", ModelEndpoint: "http://model", ModelName: "model"}
	got := WorkerOptions(cfg)
	if got.RuntimeEpoch != "epoch" || got.WorkerSocket != "w.sock" || got.WorkerName != "worker" || got.WorkerCA != "ca" || got.WorkerCert != "cert" || got.WorkerKey != "key" || got.WorkerServerName != "server" || got.EvidenceSocket != "e.sock" || got.EvidenceKey != "k" || got.ModelEndpoint != "http://model" || got.ModelName != "model" {
		t.Fatalf("options = %+v", got)
	}
}
