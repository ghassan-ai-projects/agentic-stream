package runtime_test

import (
	"path/filepath"
	"testing"

	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

func TestWorkerRuntimeConfigurationIsValidatedWithoutIO(t *testing.T) {
	t.Parallel()
	if err := runtime.ValidateWorkerRuntimeConfig(runtime.WorkerRuntimeConfig{WorkerSocket: "worker.sock"}); err == nil {
		t.Fatal("a remote route without a worker name was accepted")
	}
	if err := runtime.ValidateWorkerRuntimeConfig(runtime.WorkerRuntimeConfig{}); err != nil {
		t.Fatalf("the native default was refused: %v", err)
	}
	if _, err := runtime.NewWorkerRuntime(t.Context(), runtime.WorkerRuntimeConfig{}); err == nil {
		t.Fatal("a worker runtime without a database was accepted")
	}
}

func TestWorkerRuntimeSelectsExactlyTheConfiguredRoute(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	tests := []struct {
		name       string
		cfg        runtime.WorkerRuntimeConfig
		wantNative bool
	}{
		{"native route", runtime.WorkerRuntimeConfig{DB: db}, true},
		{"remote route never builds the native executor", runtime.WorkerRuntimeConfig{DB: db, WorkerSocket: filepath.Join(workerfake.SocketDir(t), "worker.sock"), WorkerName: "tamoz"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worker, err := runtime.NewWorkerRuntime(t.Context(), tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			_, native := worker.Executor.(*nativeexecutor.Executor)
			if worker.Executor == nil || native != tt.wantNative {
				t.Fatalf("executor = %T, want native=%v", worker.Executor, tt.wantNative)
			}
			if worker.Errors() == nil {
				t.Fatal("missing lifecycle error channel")
			}
			if err := worker.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAnAbsentWorkerRuntimeHasNoErrorsAndClosesCleanly(t *testing.T) {
	t.Parallel()
	for _, worker := range []*runtime.WorkerRuntime{nil, {}} {
		if worker.Errors() != nil || worker.Close() != nil {
			t.Fatal("an absent worker runtime must report no errors and close cleanly")
		}
	}
}
