package transport

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const evidenceTestKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestNativeBackendSelectsProviderAndScopedTools(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_MODEL_API_KEY", "test-key")
	sentinel := errors.New("constructor failure")
	for _, endpoint := range []string{"", "http://model.invalid"} {
		r := NewWorkerBackend(WorkerRuntimeConfig{ModelEndpoint: endpoint, ModelName: "model"}, func(cfg nativeexecutor.Config) (*nativeexecutor.Executor, error) {
			if endpoint == "" {
				if _, ok := cfg.Provider.(*nativeexecutor.DeterministicProvider); !ok {
					t.Fatalf("provider=%T", cfg.Provider)
				}
			} else {
				p, ok := cfg.Provider.(*nativeexecutor.OpenAICompatibleProvider)
				if !ok || p.Endpoint != endpoint || p.Model != "model" || p.APIKey != "test-key" {
					t.Fatalf("provider=%v", cfg.Provider)
				}
			}
			tools := cfg.ToolFactory(&episodes.Request{TenantID: "tenant", EntityID: "motor"})
			if len(tools) != 2 {
				t.Fatalf("tool count=%d", len(tools))
			}
			return nil, sentinel
		})
		if _, err := r.NativeExecutor(); !errors.Is(err, sentinel) {
			t.Fatalf("native error=%v", err)
		}
	}
}

func TestWorkerBackendEvidenceAndRemoteLifetime(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	socketDir, err := os.MkdirTemp("", "as-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	cfg := WorkerRuntimeConfig{DB: db, Ledger: &evidence.Ledger{DB: db}, RuntimeEpoch: "epoch", WorkerSocket: filepath.Join(t.TempDir(), "worker.sock"), WorkerName: "worker", EvidenceSocket: filepath.Join(socketDir, "evidence.sock"), EvidenceKey: evidenceTestKey}
	r := NewWorkerBackend(cfg, nativeexecutor.New)
	defer func() { _ = r.Close() }()
	secret, err := r.StartEvidence()
	if err != nil || len(secret) != 32 {
		t.Fatalf("evidence start: secret length=%d error=%v", len(secret), err)
	}
	executor, err := r.ConnectWorker(t.Context(), secret)
	if err != nil || executor == nil || r.workerConn == nil {
		t.Fatalf("worker connect: executor=%T error=%v", executor, err)
	}
	if r.Errors() == nil {
		t.Fatal("missing async errors")
	}
	if err := r.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close=%v", err)
	}
	if r.workerConn != nil || r.evidenceGRPC != nil || r.evidenceListener != nil {
		t.Fatal("resources retained after close")
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close=%v", err)
	}
	cfg.EvidenceSocket = ""
	r = NewWorkerBackend(cfg, nativeexecutor.New)
	if _, err = r.ConnectWorker(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerBackendSetupFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		cfg  WorkerRuntimeConfig
		want string
	}{
		{WorkerRuntimeConfig{EvidenceKey: "zz"}, "--evidence-key"},
		{WorkerRuntimeConfig{EvidenceKey: evidenceTestKey}, "evidence ledger is required"},
		{WorkerRuntimeConfig{EvidenceKey: evidenceTestKey, Ledger: &evidence.Ledger{}, EvidenceSocket: filepath.Join(t.TempDir(), "missing", "evidence.sock")}, "listen evidence socket"},
	} {
		r := NewWorkerBackend(tc.cfg, nativeexecutor.New)
		if _, err := r.StartEvidence(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("evidence error=%v want %s", err, tc.want)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	r := NewWorkerBackend(WorkerRuntimeConfig{WorkerCA: "/missing/ca"}, nativeexecutor.New)
	if _, err := r.ConnectWorker(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "read worker CA") {
		t.Fatalf("TLS error=%v", err)
	}
	var absent *WorkerBackend
	if absent.Errors() != nil || absent.Close() != nil {
		t.Fatal("nil backend lifecycle changed")
	}
}
