package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func startContinuousPipeline(ctx context.Context, stop context.CancelFunc, core *runtimeCore, flags serveFlags, opened effects, metrics *telemetry.Runtime, failures chan error, cleanup *cleanups) error {
	pipeline, err := core.openContinuousPipeline(ctx, stop, flags, opened, metrics, failures, cleanup)
	if err != nil {
		return err
	}
	core.pipeline = pipeline
	return startContinuousSource(ctx, stop, pipeline, flags, failures)
}

func (core *runtimeCore) openContinuousPipeline(ctx context.Context, stop context.CancelFunc, flags serveFlags, opened effects, metrics *telemetry.Runtime, failures chan error, cleanup *cleanups) (*runtime.Pipeline, error) {
	compiled, err := spec.CompileFile(ctx, flags.specPath)
	if err != nil {
		return nil, fmt.Errorf("compile spec: %w", err)
	}
	workerRuntime, err := core.openWorkerRuntime(ctx, flags.worker, cleanup)
	if err != nil {
		return nil, err
	}
	registerWorkerMonitor(ctx, workerRuntime.Errors(), failures, stop, cleanup)
	pipeline, err := core.startPipeline(ctx, compiled, flags.tenantID, workerRuntime, opened, metrics, flags.demoMode, cleanup)
	if err != nil {
		return nil, err
	}
	return pipeline, nil
}

func registerWorkerMonitor(ctx context.Context, workerErrors <-chan error, failures chan error, stop context.CancelFunc, cleanup *cleanups) {
	workerMonitorDone := monitorWorkerRuntimeErrors(ctx, workerErrors, failures, stop)
	cleanup.add(func() {
		stop()
		<-workerMonitorDone
	})
}

func startContinuousSource(ctx context.Context, stop context.CancelFunc, pipeline *runtime.Pipeline, flags serveFlags, failures chan error) error {
	run := func(name string, source func(context.Context) error) { go runSource(ctx, stop, failures, name, source) }
	if flags.liveSocket == "" {
		run("continuous pipeline", func(ctx context.Context) error {
			return pollTrace(ctx, pipeline, flags.traceFormat, flags.tracePath, flags.pollInterval)
		})
		return nil
	}
	run("live socket pipeline", func(ctx context.Context) error { return pipeline.RunLiveSocket(ctx, flags.liveSocket) })
	run("pipeline clock", func(ctx context.Context) error { return pipeline.AdvanceEvery(ctx, flags.pollInterval) })
	run("episode loop", func(ctx context.Context) error { return pipeline.RunEpisodesEvery(ctx, flags.pollInterval) })
	return waitForLiveSocket(ctx, flags.liveSocket, failures)
}

func runSource(ctx context.Context, stop context.CancelFunc, failures chan error, name string, source func(context.Context) error) {
	if err := source(ctx); err != nil && !errors.Is(err, context.Canceled) {
		failures <- fmt.Errorf("%s: %w", name, err)
		stop()
	}
}

func waitForLiveSocket(ctx context.Context, path string, failures <-chan error) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := liveSocketReady(ctx, path)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if done, err := awaitSocketPoll(ctx, ticker, failures); done {
			return err
		}
	}
}

func liveSocketReady(ctx context.Context, path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		dialer := net.Dialer{Timeout: 50 * time.Millisecond}
		conn, dialErr := dialer.DialContext(ctx, "unix", path)
		if dialErr == nil {
			_ = conn.Close()
			return true, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect live socket: %w", err)
	}
	return false, nil
}

func awaitSocketPoll(ctx context.Context, ticker *time.Ticker, failures <-chan error) (bool, error) {
	select {
	case err := <-failures:
		return true, err
	case <-ctx.Done():
		return true, pendingPipelineError(failures)
	case <-ticker.C:
	}
	return false, nil
}

func pendingPipelineError(failures <-chan error) error {
	select {
	case err := <-failures:
		return err
	default:
		return nil
	}
}
