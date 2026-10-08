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

// startContinuousPipeline starts the worker runtime and pipeline, then feeds
// it from the live socket or by polling the trace file. A source failure is
// reported on failures and stops the process.
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
	if flags.liveSocket != "" {
		go runLiveSocketSource(ctx, stop, pipeline, flags.liveSocket, failures)
		go runPipelineClock(ctx, stop, pipeline, flags.pollInterval, failures)
		return waitForLiveSocket(ctx, flags.liveSocket, failures)
	}
	go runPollingSource(ctx, stop, pipeline, flags, failures)
	return nil
}

func runLiveSocketSource(ctx context.Context, stop context.CancelFunc, pipeline *runtime.Pipeline, path string, failures chan error) {
	if runErr := pipeline.RunLiveSocket(ctx, path); runErr != nil && !errors.Is(runErr, context.Canceled) {
		failures <- fmt.Errorf("live socket pipeline: %w", runErr)
		stop()
	}
}

// runPipelineClock advances time-driven work while the live socket is quiet.
func runPipelineClock(ctx context.Context, stop context.CancelFunc, pipeline *runtime.Pipeline, interval time.Duration, failures chan error) {
	if runErr := pipeline.AdvanceEvery(ctx, interval); runErr != nil && !errors.Is(runErr, context.Canceled) {
		failures <- fmt.Errorf("pipeline clock: %w", runErr)
		stop()
	}
}

func runPollingSource(ctx context.Context, stop context.CancelFunc, pipeline *runtime.Pipeline, flags serveFlags, failures chan error) {
	if runErr := pollTrace(ctx, pipeline, flags.traceFormat, flags.tracePath, flags.pollInterval); runErr != nil {
		failures <- fmt.Errorf("continuous pipeline: %w", runErr)
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
