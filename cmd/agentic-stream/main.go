// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Version metadata injected at build time.
var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := newRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "agentic-stream",
		Short: "Streaming-native agent runtime (Situation Runtime).",
		Long: `Agentic Stream continuously converts unbounded evidence into durable,
versioned Situations and starts bounded agent episodes only when a deterministic
cognitive scheduler decides reasoning is useful.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newConfigEffectiveCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newRunLiveCommand())
	root.AddCommand(newExportRunCommand())
	root.AddCommand(newVerifyRunCommand())

	return root
}

func newExportRunCommand() *cobra.Command {
	var dbPath, outputDir string
	var manifest runartifact.Manifest
	cmd := &cobra.Command{
		Use:   "export-run --db <runtime.db> --output <directory>",
		Short: "Export one consistent, verifiable runtime evidence artifact.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dbPath == "" || outputDir == "" {
				return fmt.Errorf("--db and --output are required")
			}
			db, err := storage.Open(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("open runtime database: %w", err)
			}
			defer func() { _ = db.Close() }()
			path, err := runartifact.Export(cmd.Context(), runartifact.Options{DB: db, OutputDir: outputDir, Manifest: manifest})
			if err != nil {
				return fmt.Errorf("export run artifact: %w", err)
			}
			cmd.Printf("run_artifact=%s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&outputDir, "output", "", "New run artifact directory")
	cmd.Flags().IntVar(&manifest.SchemaVersion, "manifest-schema-version", 1, "Run manifest schema version")
	cmd.Flags().StringVar(&manifest.RunID, "run-id", "", "Experiment/run identifier")
	cmd.Flags().StringVar(&manifest.TenantID, "tenant", "", "Tenant identifier")
	cmd.Flags().StringVar(&manifest.GitCommit, "git-commit", Commit, "Agentic Stream git commit")
	cmd.Flags().BoolVar(&manifest.GitDirty, "git-dirty", false, "Whether the source checkout was dirty")
	cmd.Flags().StringVar(&manifest.Device.Board, "board", "", "Board identity")
	cmd.Flags().StringVar(&manifest.Device.DeviceID, "device-id", "", "Device identity")
	cmd.Flags().StringVar(&manifest.Device.BootID, "boot-id", "", "Device boot identity")
	cmd.Flags().StringVar(&manifest.Device.FirmwareDigest, "firmware-digest", "", "Firmware digest")
	cmd.Flags().StringVar(&manifest.Device.CapabilityDigest, "capability-digest", "", "Capability catalog digest")
	cmd.Flags().StringVar(&manifest.Worker.PromptVersion, "prompt-version", "", "Worker prompt version")
	cmd.Flags().StringVar(&manifest.Worker.PromptDigest, "prompt-digest", "", "Worker prompt digest")
	cmd.Flags().StringVar(&manifest.Worker.DecisionSchema, "decision-schema", "", "Worker decision schema")
	cmd.Flags().StringVar(&manifest.Worker.Provider, "provider", "", "Worker provider")
	cmd.Flags().StringVar(&manifest.Worker.Model, "model", "", "Worker model")
	cmd.Flags().StringVar(&manifest.Worker.SamplingParameters, "sampling-parameters", "", "Worker sampling parameters")
	cmd.Flags().StringVar(&manifest.SpecDigest, "spec-digest", "", "SituationSpec digest")
	cmd.Flags().StringVar(&manifest.PolicyDigest, "policy-digest", "", "Policy digest")
	cmd.Flags().StringVar(&manifest.CalibrationRevision, "calibration-revision", "", "Calibration revision")
	cmd.Flags().StringVar(&manifest.WiringRevision, "wiring-revision", "", "Wiring revision")
	cmd.Flags().StringVar(&manifest.ScenarioSeed, "scenario-seed", "", "Scenario seed")
	cmd.Flags().StringVar(&manifest.WallClockStart, "start", "", "Run start timestamp")
	cmd.Flags().StringVar(&manifest.WallClockEnd, "end", "", "Run end timestamp")
	cmd.Flags().Int64Var(&manifest.MonotonicStartUS, "monotonic-start-us", 0, "Monotonic start timestamp")
	cmd.Flags().Int64Var(&manifest.MonotonicEndUS, "monotonic-end-us", 0, "Monotonic end timestamp")
	cmd.Flags().StringVar(&manifest.OperatorIdentity, "operator", "", "Operator identity")
	cmd.Flags().StringVar(&manifest.SafetyReviewReference, "safety-review-reference", "", "Safety review reference")
	cmd.Flags().StringVar(&manifest.DeclaredResult, "declared-result", "", "Operator-declared result")
	return cmd
}

func newVerifyRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "verify-run <directory>",
		Short: "Verify a previously exported run artifact.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := runartifact.Verify(args[0]); err != nil {
				return fmt.Errorf("verify run artifact: %w", err)
			}
			return nil
		},
	}
}

func newRunLiveCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "run-live --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Run one owner-scoped live Go pipeline batch.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := runLive(cmd.Context(), flags)
			if err != nil {
				return err
			}
			cmd.Printf("events_ingested=%d events_processed=%d episodes_admitted=%d episodes_executed=%d intents_evaluated=%d commands_dispatched=%d\n", report.EventsIngested, report.EventsProcessed, report.EpisodesAdmitted, report.EpisodesExecuted, report.IntentsEvaluated, report.CommandsDispatched)
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&flags.specPath, "spec", "", "SituationSpec YAML path")
	cmd.Flags().StringVar(&flags.tracePath, "trace", "", "JSONL trace path")
	cmd.Flags().StringVar(&flags.tenantID, "tenant", "default", "Tenant ID")
	cmd.Flags().StringVar(&flags.traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	flags.registerShared(cmd)
	return cmd
}

// runLive runs one owner-scoped pipeline batch over a trace file. A worker
// runtime failure takes precedence over the batch error it caused.
func runLive(ctx context.Context, flags liveFlags) (runtime.PipelineReport, error) {
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	if flags.specPath == "" || flags.tracePath == "" || flags.dbPath == "" {
		return runtime.PipelineReport{}, fmt.Errorf("--spec, --trace, and --db are required")
	}
	profileOptions := flags.profileOptions()
	if err := profileOptions.validate(true); err != nil {
		return runtime.PipelineReport{}, fmt.Errorf("validate effect profile: %w", err)
	}
	var cleanup cleanups
	defer cleanup.run()
	tracerProvider, err := configureRuntimeTelemetry(runCtx)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	cleanup.add(func() { _ = tracerProvider.Shutdown(context.Background()) })
	compiled, err := spec.CompileFile(runCtx, flags.specPath)
	if err != nil {
		return runtime.PipelineReport{}, fmt.Errorf("compile spec: %w", err)
	}
	core, err := openRuntimeCore(runCtx, flags.dbPath, time.Minute, &cleanup)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	workerRuntime, err := core.openWorkerRuntime(runCtx, flags.worker, &cleanup)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	workerFailures := make(chan error, 1)
	workerMonitorDone := monitorWorkerRuntimeErrors(runCtx, workerRuntime.Errors(), workerFailures, stop)
	cleanup.add(func() {
		stop()
		<-workerMonitorDone
	})
	metrics := telemetry.NewRuntime(time.Now().UTC())
	opened, err := core.openEffects(runCtx, profileOptions, metrics, true, &cleanup)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	pipeline, err := core.startPipeline(runCtx, compiled, flags.tenantID, workerRuntime, opened, metrics, false, &cleanup)
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	report, err := runTrace(runCtx, pipeline, flags.traceFormat, flags.tracePath)
	if workerErr := readWorkerRuntimeError(workerFailures); workerErr != nil {
		return runtime.PipelineReport{}, workerErr
	}
	if err != nil {
		return runtime.PipelineReport{}, err
	}
	return report, nil
}

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
	handler := api.NewRuntimeHandler(core.service, core.db, notify.SSEConfig{
		TenantID:  flags.tenantID,
		MaxLag:    1000,
		Authorize: notify.BearerTokenAuthorizer(subscriberToken),
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

func monitorWorkerRuntimeErrors(ctx context.Context, workerErrors <-chan error, failures chan<- error, stop context.CancelFunc) <-chan struct{} {
	done := make(chan struct{})
	if workerErrors == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
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
	}()
	return done
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

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build metadata.",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Printf("agentic-stream version %s (commit %s)\n", Version, Commit)
			cmd.Printf("contract: %s\n", contractsv1.ContractVersion)
			cmd.Printf("protocol: %s\n", contractsv1.ProtocolVersion)
		},
	}
}

func newValidateCommand() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "validate <spec.yaml>",
		Short: "Validate and compile a SituationSpec.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			result, err := spec.CompileFile(context.Background(), path)
			if err != nil {
				return fmt.Errorf("compile %s: %w", path, err)
			}

			if outputJSON {
				cmd.Printf("%s\n", result.CanonicalJSON)
				return nil
			}

			cmd.Printf("ok: %s\n", result.Metadata.Name)
			cmd.Printf("version: %s\n", result.Metadata.Version)
			cmd.Printf("digest: %s\n", result.Digest)
			cmd.Printf("schema: %s\n", result.SchemaVersion)
			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Emit canonical JSON instead of summary")
	return cmd
}

func newRunCommand() *cobra.Command {
	var (
		dbPath   string
		tenantID string
	)

	cmd := &cobra.Command{
		Use:   "run --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Replay a JSONL trace against a spec and print the canonical result.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			specPath, err := cmd.Flags().GetString("spec")
			if err != nil {
				return fmt.Errorf("get spec flag: %w", err)
			}
			tracePath, err := cmd.Flags().GetString("trace")
			if err != nil {
				return fmt.Errorf("get trace flag: %w", err)
			}
			if specPath == "" || tracePath == "" {
				return fmt.Errorf("--spec and --trace are required")
			}
			if dbPath == "" {
				dbPath = tracePath + ".replay.db"
			}

			result, err := replay.Run(cmd.Context(), dbPath, specPath, tracePath, tenantID)
			if err != nil {
				return fmt.Errorf("run replay: %w", err)
			}

			cmd.Printf("events_processed=%d situation_versions=%d versions_hash=%s\n",
				result.EventsProcessed, result.VersionCount, result.VersionsHash)
			return nil
		},
	}

	cmd.Flags().String("spec", "", "Path to the SituationSpec YAML file")
	cmd.Flags().String("trace", "", "Path to the JSONL trace file")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: <trace>.replay.db)")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "Tenant ID")

	return cmd
}

func newConfigEffectiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config effective",
		Short: "Show effective runtime configuration (placeholder).",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("config effective: not yet implemented")
		},
	}
}
