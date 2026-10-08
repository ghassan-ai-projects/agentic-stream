package runtime

import (
	"path/filepath"
	"testing"

	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestWorkerRuntimeFacade(t *testing.T) {
	t.Parallel()
	if err := ValidateWorkerRuntimeConfig(WorkerRuntimeConfig{WorkerSocket: "worker.sock"}); err == nil {
		t.Fatal("invalid options accepted")
	}
	if _, err := NewWorkerRuntime(t.Context(), WorkerRuntimeConfig{}); err == nil {
		t.Fatal("missing database accepted")
	}
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, remote := range []bool{false, true} {
		cfg := WorkerRuntimeConfig{DB: db}
		if remote {
			cfg.WorkerSocket = filepath.Join(t.TempDir(), "worker.sock")
			cfg.WorkerName = "tamoz"
		}
		r, err := NewWorkerRuntime(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, native := r.Executor.(*nativeexecutor.Executor)
		if r.Executor == nil || native == remote {
			t.Fatalf("remote=%v executor=%T", remote, r.Executor)
		}
		if r.Errors() == nil {
			t.Fatal("missing lifecycle error channel")
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []*WorkerRuntime{nil, {}} {
		if r.Errors() != nil || r.Close() != nil {
			t.Fatal("empty facade lifecycle changed")
		}
	}
}
