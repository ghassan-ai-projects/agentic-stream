package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestValidateWorkerRuntimeConfig(t *testing.T) {
	validKey := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	tests := []struct {
		name    string
		config  WorkerRuntimeConfig
		wantErr string
	}{
		{name: "native defaults", config: WorkerRuntimeConfig{}},
		{name: "evidence requires worker", config: WorkerRuntimeConfig{EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}, wantErr: "--evidence-socket requires --worker-socket"},
		{name: "evidence key requires socket", config: WorkerRuntimeConfig{EvidenceKey: validKey}, wantErr: "--evidence-key requires --evidence-socket"},
		{name: "evidence key is bounded", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: "00"}, wantErr: "--evidence-key must be at least 32 bytes of hex"},
		{name: "worker name is required", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock"}, wantErr: "--worker-name is required with --worker-socket"},
		{name: "mTLS is all or nothing", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem"}, wantErr: "--worker-ca, --worker-cert, and --worker-key are required together"},
		{name: "mTLS requires worker", config: WorkerRuntimeConfig{WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local"}, wantErr: "worker TLS flags require --worker-socket"},
		{name: "mTLS server name is required", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem"}, wantErr: "--worker-server-name is required with mTLS"},
		{name: "server name requires mTLS", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerServerName: "worker.local"}, wantErr: "--worker-server-name requires mTLS"},
		{name: "model name is required", config: WorkerRuntimeConfig{ModelEndpoint: "http://model"}, wantErr: "--model-name is required with --model-endpoint"},
		{name: "complete worker and evidence", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkerRuntimeConfig(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateWorkerRuntimeConfig() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ValidateWorkerRuntimeConfig() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// P1 gate 8 (B1): on an ExecutorName=tamoz route (WorkerSocket configured) the
// Go native executor is NEVER constructed — the constructor is injectable so
// the test can count invocations, and the count must stay zero even when the
// worker dial itself fails.
func TestExecutorSelectionSkipsNativeOnTamoz(t *testing.T) {
	orig := newNativeExecutor
	calls := 0
	newNativeExecutor = func(nativeexecutor.Config) (*nativeexecutor.Executor, error) {
		calls++
		return nil, errors.New("native executor must not be constructed")
	}
	defer func() { newNativeExecutor = orig }()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "tamoz.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer func() { _ = db.Close() }()
	// The gRPC dial is lazy, so the constructor succeeds without a live
	// worker — the adversarial assertion is that the native executor was
	// never constructed and the route holds the worker executor.
	r, err := NewWorkerRuntime(ctx, WorkerRuntimeConfig{
		DB:           db,
		WorkerSocket: filepath.Join(t.TempDir(), "worker.sock"),
		WorkerName:   "tamoz",
	})
	if err != nil {
		t.Fatalf("unexpected constructor error on the tamoz route: %v", err)
	}
	if calls != 0 {
		t.Fatalf("native executor constructed %d times on a tamoz route; want 0", calls)
	}
	if _, ok := r.Executor.(*nativeexecutor.Executor); ok {
		t.Fatalf("the tamoz route holds a native executor")
	}
	if r.Executor == nil {
		t.Fatalf("the tamoz route must hold the worker executor")
	}
}

// The native route still constructs exactly once — the gate must not weaken
// the native path.
func TestNativeModeConstructsTheNativeExecutor(t *testing.T) {
	orig := newNativeExecutor
	calls := 0
	newNativeExecutor = func(cfg nativeexecutor.Config) (*nativeexecutor.Executor, error) {
		calls++
		return nil, errors.New("probe constructor")
	}
	defer func() { newNativeExecutor = orig }()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer func() { _ = db.Close() }()
	_, err = NewWorkerRuntime(ctx, WorkerRuntimeConfig{DB: db})
	if err == nil {
		t.Fatal("expected the probed native constructor error")
	}
	if calls != 1 {
		t.Fatalf("native executor constructed %d times in native mode; want 1", calls)
	}
}
