package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/spf13/cobra"
)

// serveFlags are the serve command's flags.
type serveFlags struct {
	liveFlags
	listenAddress, liveSocket string
	ownerLease, pollInterval  time.Duration
	demoMode                  bool
}

func newServeCommand() *cobra.Command {
	var flags serveFlags
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the live Go runtime and readiness endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), flags)
		},
	}
	cmd.Flags().StringVar(&flags.dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&flags.specPath, "spec", "", "SituationSpec YAML path for continuous ingestion")
	cmd.Flags().StringVar(&flags.tracePath, "trace", "", "append-only JSONL trace path for continuous ingestion")
	cmd.Flags().StringVar(&flags.liveSocket, "live-socket", "", "Unix socket for live normalized JSONL telemetry ingestion")
	cmd.Flags().StringVar(&flags.traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	flags.registerShared(cmd)
	cmd.Flags().StringVar(&flags.tenantID, "tenant", "default", "tenant served by this runtime process")
	cmd.Flags().StringVar(&flags.listenAddress, "listen", "127.0.0.1:8080", "loopback HTTP listen address")
	cmd.Flags().DurationVar(&flags.ownerLease, "owner-lease", time.Minute, "runtime owner lease duration")
	cmd.Flags().DurationVar(&flags.pollInterval, "poll-interval", time.Second, "continuous source polling interval")
	cmd.Flags().BoolVar(&flags.demoMode, "demo-mode", false, "admit fixture executors (demos and tests only; a production route never admits fixture)")
	return cmd
}

// validate checks every serve input before a database, socket, or credential
// is opened, and returns the notification subscriber token.
func (f serveFlags) validate() (string, error) {
	if f.dbPath == "" {
		return "", fmt.Errorf("--db is required")
	}
	if err := validateServeSources(f.specPath, f.tracePath, f.liveSocket, f.worker.WorkerSocket); err != nil {
		return "", err
	}
	if f.liveSocket != "" && f.traceFormat != "normalized" {
		return "", fmt.Errorf("--live-socket requires --trace-format normalized")
	}
	if err := f.profileOptions().validate(f.tracePath != ""); err != nil {
		return "", fmt.Errorf("validate effect profile: %w", err)
	}
	if err := runtime.ValidateWorkerRuntimeConfig(f.worker); err != nil {
		return "", fmt.Errorf("worker runtime config: %w", err)
	}
	if f.pollInterval <= 0 {
		return "", fmt.Errorf("--poll-interval must be positive")
	}
	subscriberToken := os.Getenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN")
	if subscriberToken == "" {
		return "", fmt.Errorf("AGENTIC_STREAM_SUBSCRIBER_TOKEN is required for notification subscribers")
	}
	if !isLoopbackListenAddress(f.listenAddress) {
		return "", fmt.Errorf("non-loopback --listen requires an authenticated deployment proxy")
	}
	return subscriberToken, nil
}

// serve runs the runtime HTTP surface and, when a spec is configured, the
// continuous pipeline, until ctx ends or a source fails.
func serve(ctx context.Context, flags serveFlags) error {
	subscriberToken, err := flags.validate()
	if err != nil {
		return err
	}
	var cleanup cleanups
	defer cleanup.run()
	core, err := openRuntimeCore(ctx, flags.dbPath, flags.ownerLease, &cleanup)
	if err != nil {
		return err
	}
	metrics := telemetry.NewRuntime(time.Now().UTC())
	tracerProvider, err := configureRuntimeTelemetry(ctx)
	if err != nil {
		return err
	}
	cleanup.add(func() { _ = tracerProvider.Shutdown(context.Background()) })
	runCtx, stop := context.WithCancel(ctx)
	cleanup.add(stop)
	opened, err := core.openEffects(runCtx, flags.profileOptions(), metrics, flags.tracePath != "", &cleanup)
	if err != nil {
		return err
	}
	pipelineErrors := make(chan error, 1)
	if flags.specPath != "" {
		if err := startContinuousPipeline(runCtx, stop, core, flags, opened, metrics, pipelineErrors, &cleanup); err != nil {
			return err
		}
	}
	handler := api.NewRuntimeHandler(core.service, core.db, api.SSEConfig{
		TenantID:  flags.tenantID,
		MaxLag:    1000,
		Authorize: api.BearerTokenAuthorizer(subscriberToken),
	}, metrics.Handler(), core.epochControl, core.epoch, os.Getenv("AGENTIC_STREAM_CONTROL_TOKEN"))
	if err := serveHTTP(runCtx, flags.listenAddress, handler); err != nil {
		return err
	}
	select {
	case pipelineErr := <-pipelineErrors:
		return pipelineErr
	default:
		return nil
	}
}

// startContinuousPipeline starts the worker runtime and pipeline, then feeds
// it from the live socket or by polling the trace file. A source failure is
// reported on failures and stops the process.
func startContinuousPipeline(ctx context.Context, stop context.CancelFunc, core *runtimeCore, flags serveFlags, opened effects, metrics *telemetry.Runtime, failures chan error, cleanup *cleanups) error {
	compiled, err := spec.CompileFile(ctx, flags.specPath)
	if err != nil {
		return fmt.Errorf("compile spec: %w", err)
	}
	workerRuntime, err := core.openWorkerRuntime(ctx, flags.worker, cleanup)
	if err != nil {
		return err
	}
	workerMonitorDone := monitorWorkerRuntimeErrors(ctx, workerRuntime.Errors(), failures, stop)
	cleanup.add(func() {
		stop()
		<-workerMonitorDone
	})
	pipeline, err := core.startPipeline(ctx, compiled, flags.tenantID, workerRuntime, opened, metrics, flags.demoMode, cleanup)
	if err != nil {
		return err
	}
	if flags.liveSocket != "" {
		go func() {
			if runErr := pipeline.RunLiveSocket(ctx, flags.liveSocket); runErr != nil && !errors.Is(runErr, context.Canceled) {
				failures <- fmt.Errorf("live socket pipeline: %w", runErr)
				stop()
			}
		}()
		return waitForLiveSocket(ctx, flags.liveSocket, failures)
	}
	go func() {
		if runErr := pollTrace(ctx, pipeline, flags.traceFormat, flags.tracePath, flags.pollInterval); runErr != nil {
			failures <- fmt.Errorf("continuous pipeline: %w", runErr)
			stop()
		}
	}()
	return nil
}

// serveHTTP serves handler until ctx ends, then shuts down gracefully.
func serveHTTP(ctx context.Context, address string, handler http.Handler) error {
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		// ctx is already done; keep its values but give shutdown its own deadline.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve runtime: %w", err)
	}
	return nil
}

func validateServeSources(specPath, tracePath, liveSocket, workerSocket string) error {
	if tracePath != "" && liveSocket != "" {
		return fmt.Errorf("--trace and --live-socket are mutually exclusive")
	}
	continuousSource := tracePath != "" || liveSocket != ""
	if (specPath == "") != !continuousSource {
		if liveSocket == "" {
			return fmt.Errorf("--spec and --trace must be provided together for continuous ingestion")
		}
		return fmt.Errorf("--spec and --live-socket must be provided together for continuous ingestion")
	}
	if workerSocket != "" && !continuousSource {
		return fmt.Errorf("--worker-socket requires --spec and (--trace or --live-socket) for continuous ingestion")
	}
	return nil
}

func waitForLiveSocket(ctx context.Context, path string, failures <-chan error) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			dialer := net.Dialer{Timeout: 50 * time.Millisecond}
			conn, dialErr := dialer.DialContext(ctx, "unix", path)
			if dialErr == nil {
				_ = conn.Close()
				return nil
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect live socket: %w", err)
		}
		select {
		case err := <-failures:
			return err
		case <-ctx.Done():
			select {
			case err := <-failures:
				return err
			default:
				return nil
			}
		case <-ticker.C:
		}
	}
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
