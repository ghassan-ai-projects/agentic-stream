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

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
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
	db := storagetest.OpenTemp(t)

	socketDir, err := os.MkdirTemp("", "as-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	cfg := WorkerRuntimeConfig{DB: db, Ledger: testEvidenceLedger(t, db), RuntimeEpoch: "epoch", WorkerSocket: filepath.Join(t.TempDir(), "worker.sock"), WorkerName: "worker", EvidenceSocket: filepath.Join(socketDir, "evidence.sock"), EvidenceKey: evidenceTestKey}
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
	socketDir, err := os.MkdirTemp("", "as-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	occupiedPath := filepath.Join(socketDir, "evidence.sock")
	if err := os.WriteFile(occupiedPath, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		cfg  WorkerRuntimeConfig
		want string
	}{
		{"invalid key", WorkerRuntimeConfig{EvidenceKey: "zz"}, "--evidence-key"},
		{"missing ledger", WorkerRuntimeConfig{EvidenceKey: evidenceTestKey}, "evidence ledger is required"},
		{"occupied socket path", WorkerRuntimeConfig{EvidenceKey: evidenceTestKey, Ledger: testEvidenceLedger(t, nil), EvidenceSocket: occupiedPath}, "listen evidence socket: refusing unsafe existing socket path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewWorkerBackend(tc.cfg, nativeexecutor.New)
			t.Cleanup(func() {
				if err := r.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err := r.StartEvidence(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("evidence error=%v want %s", err, tc.want)
			}
		})
	}
	if content, err := os.ReadFile(occupiedPath); err != nil || string(content) != "existing file" {
		t.Fatalf("occupied path was changed: content=%q error=%v", content, err)
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

func testEvidenceLedger(t *testing.T, db *storage.DB) *evidence.Service {
	t.Helper()
	if db == nil {
		var err error
		db, err = storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "evidence.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
	}
	service, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{DB: db, LeaseOwner: "fixture", RuntimeEpoch: "epoch", OwnerCheck: func(context.Context, *sql.Tx, string) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestEvidenceCapabilityFactoryUsesTheEvidenceReadBudget(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	factory := evidenceCapabilityFactory(nil, "epoch-1", now)
	if factory.MaxRows != evidence.DefaultReadMaxRows || factory.MaxBytes != evidence.DefaultReadMaxBytes {
		t.Errorf("budget = %d rows, %d bytes", factory.MaxRows, factory.MaxBytes)
	}
	if !factory.From.Equal(now.Add(-evidence.DefaultReadWindow)) || !factory.Until.Equal(now.Add(evidence.DefaultReadWindow)) {
		t.Errorf("window = %v..%v", factory.From, factory.Until)
	}
}
