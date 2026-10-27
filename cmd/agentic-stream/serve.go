package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type serveFlags struct {
	liveFlags
	plantWire                 plantWireFlags
	listenAddress, liveSocket string
	playbookPath              string
	draftAfter                int
	ownerLease, pollInterval  time.Duration
	demoMode                  bool
}

func newServeCommand() *cobra.Command {
	var flags serveFlags
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the live Go runtime and readiness endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			schemas, err := loadLoggedEventSchemas(cmd)
			if err != nil {
				return err
			}
			flags.eventSchemas = schemas
			return serve(cmd.Context(), flags)
		},
	}
	registerServeFlags(cmd, &flags)
	return cmd
}

func registerServeFlags(cmd *cobra.Command, flags *serveFlags) {
	cmd.Flags().StringVar(&flags.dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&flags.specPath, "spec", "", "SituationSpec YAML path for continuous ingestion")
	cmd.Flags().StringVar(&flags.playbookPath, "playbook", "", "playbook YAML path compiled against --spec at start; its rules answer the cases they cover without a model call, and approved rules are written to it")
	cmd.Flags().IntVar(&flags.draftAfter, "draft-after", 3, "verified, uncontradicted cases of one key that ask the worker to draft a playbook rule")
	cmd.Flags().StringVar(&flags.tracePath, "trace", "", "append-only JSONL trace path for continuous ingestion")
	cmd.Flags().StringVar(&flags.liveSocket, "live-socket", "", "Unix socket for live normalized JSONL telemetry ingestion")
	cmd.Flags().StringVar(&flags.traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	flags.registerShared(cmd)
	cmd.Flags().StringVar(&flags.tenantID, "tenant", contractsv1.TenantID, "tenant served by this runtime process")
	cmd.Flags().StringVar(&flags.listenAddress, "listen", "127.0.0.1:8080", "loopback HTTP listen address")
	cmd.Flags().DurationVar(&flags.ownerLease, "owner-lease", sources.DefaultLease, "runtime owner lease duration")
	cmd.Flags().DurationVar(&flags.pollInterval, "poll-interval", time.Second, "interval for polling a --trace source and for advancing timers, debounced cognition and approved commands while a --live-socket is quiet")
	cmd.Flags().BoolVar(&flags.demoMode, "demo-mode", false, "admit fixture executors (demos and tests only; a production route never admits fixture)")
	cmd.Flags().DurationVar(&flags.effect.DispatchLease, "dispatch-lease", 0, "lease of one approved command's dispatch; a device call still open after nine tenths of it becomes an unknown outcome (default: the one-minute default lease)")
	registerPlantWireFlags(cmd, &flags.effect.PlantWireSocket, &flags.plantWire)
}

func (f serveFlags) validate() (string, error) {
	if f.dbPath == "" {
		return "", fmt.Errorf("--db is required")
	}
	if err := f.validateSources(); err != nil {
		return "", err
	}
	if err := f.validateEffects(); err != nil {
		return "", err
	}
	if err := runtime.ValidateWorkerRuntimeConfig(f.worker); err != nil {
		return "", fmt.Errorf("worker runtime config: %w", err)
	}
	return f.validateOperatorAccess()
}

func (f serveFlags) validateEffects() error {
	if f.effect.DispatchLease < 0 {
		return fmt.Errorf("--dispatch-lease must not be negative")
	}
	if err := f.profileOptions().validate(f.tracePath != ""); err != nil {
		return fmt.Errorf("validate effect profile: %w", err)
	}
	return f.plantWire.validate(f.profileOptions())
}

func (f serveFlags) validateSources() error {
	if err := validateServeSources(f.specPath, f.tracePath, f.liveSocket, f.effect.PlantWireSocket, f.worker.WorkerSocket); err != nil {
		return err
	}
	if f.playbookPath != "" && f.specPath == "" {
		return fmt.Errorf("--playbook requires --spec")
	}
	if f.liveSocket != "" && f.traceFormat != "normalized" {
		return fmt.Errorf("--live-socket requires --trace-format normalized")
	}
	return nil
}

func (f serveFlags) validateSubscriberAccess() (string, error) {
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

func validateServeSources(specPath, tracePath, liveSocket, plantWireSocket, workerSocket string) error {
	if tracePath != "" && (liveSocket != "" || plantWireSocket != "") {
		return fmt.Errorf("--trace is mutually exclusive with --live-socket and --plant-wire-socket")
	}
	continuousSource := tracePath != "" || liveSocket != "" || plantWireSocket != ""
	if (specPath == "") != !continuousSource {
		return fmt.Errorf("--spec and one of --trace, --live-socket or --plant-wire-socket must be provided together for continuous ingestion")
	}
	if workerSocket != "" && !continuousSource {
		return fmt.Errorf("--worker-socket requires --spec and a continuous source (--trace, --live-socket or --plant-wire-socket)")
	}
	return nil
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

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
	return core.serveRuntime(ctx, flags, subscriberToken, &cleanup)
}

func (core *runtimeCore) serveRuntime(ctx context.Context, flags serveFlags, subscriberToken string, cleanup *cleanups) error {
	metrics := telemetry.NewRuntime(time.Now().UTC())
	tracerProvider, err := configureRuntimeTelemetry(ctx)
	if err != nil {
		return err
	}
	cleanup.add(func() { _ = tracerProvider.Shutdown(context.Background()) })
	runCtx, stop := context.WithCancel(ctx)
	cleanup.add(stop)
	opened, err := core.openServeEffects(runCtx, flags, metrics, cleanup)
	if err != nil {
		return err
	}
	return core.servePipeline(runCtx, stop, flags, opened, metrics, subscriberToken, cleanup)
}

func (core *runtimeCore) servePipeline(runCtx context.Context, stop context.CancelFunc, flags serveFlags, opened effects, metrics *telemetry.Runtime, subscriberToken string, cleanup *cleanups) error {
	pipelineErrors := make(chan error, 1)
	if flags.specPath != "" {
		if err := startContinuousPipeline(runCtx, stop, core, flags, opened, metrics, pipelineErrors, cleanup); err != nil {
			return err
		}
	}
	handler := core.runtimeHandler(flags, subscriberToken, metrics)
	slog.Info("serving runtime API", "version", Version, "commit", Commit, "listen", flags.listenAddress, "db", flags.dbPath,
		"tenant", flags.tenantID, "epoch", core.epoch, "effect_profile", flags.effectProfile, "worker_socket", flags.worker.WorkerSocket)
	if err := serveHTTP(runCtx, flags.listenAddress, handler); err != nil {
		return err
	}
	return pendingPipelineError(pipelineErrors)
}

func (core *runtimeCore) runtimeHandler(flags serveFlags, subscriberToken string, metrics *telemetry.Runtime) http.Handler {
	return core.approvalHandler(api.NewRuntimeHandler(core.service, core.db, api.SSEConfig{
		TenantID:  flags.tenantID,
		MaxLag:    1000,
		Authorize: api.BearerTokenAuthorizer(subscriberToken),
	}, telemetry.MetricsHandler(metrics, core.healthGauges()), core.epochControl, core.epoch, os.Getenv("AGENTIC_STREAM_CONTROL_TOKEN")))
}

func serveHTTP(ctx context.Context, address string, handler http.Handler) error {
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := sources.DetachedContext(ctx)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve runtime: %w", err)
	}
	return nil
}

func (core *runtimeCore) healthGauges() telemetry.GaugeSource {
	if core.pipeline == nil {
		return nil
	}
	return core.pipeline.HealthGauges
}
