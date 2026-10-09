package composition

import (
	"errors"
	"path/filepath"
	"testing"

	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

func TestWorkerCompositionValidatesBeforeOpeningResources(t *testing.T) {
	t.Parallel()
	calls := 0
	sentinel := errors.New("native construction probe")
	constructor := func(nativeexecutor.Config) (*nativeexecutor.Executor, error) { calls++; return nil, sentinel }
	if _, err := NewWorkerRuntime(t.Context(), transport.WorkerRuntimeConfig{}); err == nil || err.Error() != "worker runtime database is required" {
		t.Fatalf("public database validation=%v", err)
	}
	if _, err := newWorkerRuntime(t.Context(), transport.WorkerRuntimeConfig{}, constructor); err == nil || err.Error() != "worker runtime database is required" {
		t.Fatalf("database validation=%v", err)
	}
	db := storagetest.OpenTemp(t)

	if _, err := newWorkerRuntime(t.Context(), transport.WorkerRuntimeConfig{DB: db, WorkerSocket: "worker"}, constructor); err == nil || calls != 0 {
		t.Fatalf("option validation=%v native calls=%d", err, calls)
	}
	if _, err := newWorkerRuntime(t.Context(), transport.WorkerRuntimeConfig{DB: db}, constructor); !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("native construction=%v calls=%d", err, calls)
	}
	r, err := newWorkerRuntime(t.Context(), transport.WorkerRuntimeConfig{DB: db, WorkerSocket: filepath.Join(workerfake.SocketDir(t), "worker.sock"), WorkerName: "worker"}, constructor)
	if err != nil || r.Executor == nil || calls != 1 {
		t.Fatalf("remote construction=%v calls=%d", err, calls)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
