package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
)

type WorkerBackend interface {
	NativeExecutor() (episodes.Executor, error)
	StartEvidence() ([]byte, error)
	ConnectWorker(context.Context, []byte) (episodes.Executor, error)
	Errors() <-chan error
	Drafter() draftport.Drafter
	Close() error
}

type WorkerRuntime struct {
	Executor episodes.Executor
	backend  WorkerBackend
}

func NewWorkerRuntime(ctx context.Context, cfg domain.WorkerOptions, backend WorkerBackend) (*WorkerRuntime, error) {
	r := &WorkerRuntime{backend: backend}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			_ = r.Close()
		}
	}()

	if err := r.configureExecutor(ctx, cfg); err != nil {
		return nil, err
	}
	cleanupOnError = false
	return r, nil
}

func (r *WorkerRuntime) configureExecutor(ctx context.Context, cfg domain.WorkerOptions) error {

	if cfg.WorkerSocket == "" {
		nativeExecutor, err := r.backend.NativeExecutor()
		if err != nil {
			return err
		}
		r.Executor = nativeExecutor
	}
	return r.configureWorkerEvidence(ctx, cfg)
}

func (r *WorkerRuntime) configureWorkerEvidence(ctx context.Context, cfg domain.WorkerOptions) error {
	var evidenceSecret []byte
	if cfg.EvidenceSocket != "" {
		var err error
		if evidenceSecret, err = r.backend.StartEvidence(); err != nil {
			return err
		}
	}
	if cfg.WorkerSocket != "" {
		executor, err := r.backend.ConnectWorker(ctx, evidenceSecret)
		if err != nil {
			return err
		}
		r.Executor = executor
	}
	return nil
}

func (r *WorkerRuntime) Drafter() draftport.Drafter {
	if r == nil || r.backend == nil {
		return nil
	}
	return r.backend.Drafter()
}

func (r *WorkerRuntime) Errors() <-chan error {
	if r == nil || r.backend == nil {
		return nil
	}
	return r.backend.Errors()
}

func (r *WorkerRuntime) Close() error {
	if r == nil || r.backend == nil {
		return nil
	}
	return r.backend.Close()
}
