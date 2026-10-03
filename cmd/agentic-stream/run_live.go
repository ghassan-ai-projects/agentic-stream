package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func newRunLiveCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "run-live --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Run one owner-scoped live Go pipeline batch.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printLiveBatch(cmd, flags)
		},
	}
	registerLiveBatchFlags(cmd, &flags)
	return cmd
}

func registerLiveBatchFlags(cmd *cobra.Command, flags *liveFlags) {
	cmd.Flags().StringVar(&flags.dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&flags.specPath, "spec", "", "SituationSpec YAML path")
	cmd.Flags().StringVar(&flags.tracePath, "trace", "", "JSONL trace path")
	cmd.Flags().StringVar(&flags.tenantID, "tenant", "default", "Tenant ID")
	cmd.Flags().StringVar(&flags.traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	flags.registerShared(cmd)
}

func printLiveBatch(cmd *cobra.Command, flags liveFlags) error {
	report, err := runLive(cmd.Context(), flags)
	if err != nil {
		return err
	}
	cmd.Printf("events_ingested=%d events_processed=%d episodes_admitted=%d episodes_executed=%d intents_evaluated=%d commands_dispatched=%d\n", report.EventsIngested, report.EventsProcessed, report.EpisodesAdmitted, report.EpisodesExecuted, report.IntentsEvaluated, report.CommandsDispatched)
	return nil
}

// runLive runs one owner-scoped pipeline batch over a trace file. A worker
// runtime failure takes precedence over the batch error it caused.
func runLive(ctx context.Context, flags liveFlags) (runtime.PipelineReport, error) {
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	if err := validateLiveBatch(flags); err != nil {
		return runtime.PipelineReport{}, err
	}
	var cleanup cleanups
	defer cleanup.run()
	pipeline, workerFailures, err := prepareLiveBatch(runCtx, flags, stop, &cleanup)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	return executeLiveTrace(runCtx, pipeline, flags.traceFormat, flags.tracePath, workerFailures)
}

func validateLiveBatch(flags liveFlags) error {
	if flags.specPath == "" || flags.tracePath == "" || flags.dbPath == "" {
		return fmt.Errorf("--spec, --trace, and --db are required")
	}
	profileOptions := flags.profileOptions()
	if err := profileOptions.validate(true); err != nil {
		return fmt.Errorf("validate effect profile: %w", err)
	}
	return nil
}

func prepareLiveBatch(runCtx context.Context, flags liveFlags, stop context.CancelFunc, cleanup *cleanups) (*runtime.Pipeline, <-chan error, error) {
	if err := startLiveTelemetry(runCtx, cleanup); err != nil {
		return nil, nil, err
	}
	compiled, err := spec.CompileFile(runCtx, flags.specPath)
	if err != nil {
		return nil, nil, fmt.Errorf("compile spec: %w", err)
	}
	core, err := openRuntimeCore(runCtx, flags.dbPath, time.Minute, cleanup)
	if err != nil {
		return nil, nil, err
	}
	return core.prepareLiveExecution(runCtx, flags, compiled, stop, cleanup)
}

func (core *runtimeCore) prepareLiveExecution(runCtx context.Context, flags liveFlags, compiled *spec.CompiledSpec, stop context.CancelFunc, cleanup *cleanups) (*runtime.Pipeline, <-chan error, error) {
	workerRuntime, err := core.openWorkerRuntime(runCtx, flags.worker, cleanup)
	if err != nil {
		return nil, nil, err
	}
	workerFailures := startLiveWorkerMonitor(runCtx, workerRuntime.Errors(), stop, cleanup)
	metrics := telemetry.NewRuntime(time.Now().UTC())
	opened, err := core.openEffects(runCtx, flags.profileOptions(), metrics, true, cleanup)
	if err != nil {
		return nil, nil, err
	}
	pipeline, err := core.startPipeline(runCtx, compiled, flags.tenantID, workerRuntime, opened, metrics, false, cleanup)
	if err != nil {
		return nil, nil, err
	}
	return pipeline, workerFailures, nil
}

func executeLiveTrace(ctx context.Context, pipeline *runtime.Pipeline, format, path string, workerFailures <-chan error) (runtime.PipelineReport, error) {
	report, err := runTrace(ctx, pipeline, format, path)
	if workerErr := readWorkerRuntimeError(workerFailures); workerErr != nil {
		return runtime.PipelineReport{}, workerErr
	}
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	return report, nil
}

func startLiveTelemetry(ctx context.Context, cleanup *cleanups) error {
	tracerProvider, err := configureRuntimeTelemetry(ctx)
	if err != nil {
		return err
	}
	cleanup.add(func() { _ = tracerProvider.Shutdown(context.Background()) })
	return nil
}

func startLiveWorkerMonitor(ctx context.Context, workerErrors <-chan error, stop context.CancelFunc, cleanup *cleanups) <-chan error {
	workerFailures := make(chan error, 1)
	workerMonitorDone := monitorWorkerRuntimeErrors(ctx, workerErrors, workerFailures, stop)
	cleanup.add(func() {
		stop()
		<-workerMonitorDone
	})
	return workerFailures
}

func monitorWorkerRuntimeErrors(ctx context.Context, workerErrors <-chan error, failures chan<- error, stop context.CancelFunc) <-chan struct{} {
	done := make(chan struct{})
	if workerErrors == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		relayWorkerRuntimeError(ctx, workerErrors, failures, stop)
	}()
	return done
}

func relayWorkerRuntimeError(ctx context.Context, workerErrors <-chan error, failures chan<- error, stop context.CancelFunc) {
	select {
	case workerErr := <-workerErrors:
		if workerErr == nil {
			return
		}
		select {
		case failures <- fmt.Errorf("worker runtime: %w", workerErr):
		default:
		}
		stop()
	case <-ctx.Done():
	}
}

func readWorkerRuntimeError(failures <-chan error) error {
	if failures == nil {
		return nil
	}
	select {
	case workerErr := <-failures:
		return workerErr
	default:
		return nil
	}
}

func configureRuntimeTelemetry(ctx context.Context) (interface{ Shutdown(context.Context) error }, error) {
	provider, err := telemetry.Configure(ctx, "agentic-stream", telemetryEndpoint())
	if err != nil {
		return nil, fmt.Errorf("configure OpenTelemetry: %w", err)
	}
	return provider, nil
}

func telemetryEndpoint() string {
	if endpoint := os.Getenv("AGENTIC_STREAM_OTLP_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
}
