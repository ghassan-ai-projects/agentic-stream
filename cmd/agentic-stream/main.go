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

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
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
	var dbPath, specPath, tracePath, tenantID, workerSocket, workerName, traceFormat, effectProfile string
	var modelEndpoint, modelName string
	var workerCA, workerCert, workerKey, workerServerName, evidenceSocket, evidenceKey string
	var deviceSocket, deviceCatalog string
	var deviceFirmwareDigests []string
	var liveActuation, ownerAuthorized bool
	cmd := &cobra.Command{
		Use:   "run-live --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Run one owner-scoped live Go pipeline batch.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if specPath == "" || tracePath == "" || dbPath == "" {
				return fmt.Errorf("--spec, --trace, and --db are required")
			}
			profileOptions := effectProfileOptions{
				Profile: actions.EffectProfile(effectProfile), DeviceSocket: deviceSocket, DeviceCatalog: deviceCatalog,
				AllowedFirmwareDigests: deviceFirmwareDigests, LiveActuation: liveActuation,
				OwnerAuthorized: ownerAuthorized,
			}
			if err := profileOptions.validate(true); err != nil {
				return fmt.Errorf("validate effect profile: %w", err)
			}
			tracerProvider, telemetryErr := configureRuntimeTelemetry(cmd.Context())
			if telemetryErr != nil {
				return telemetryErr
			}
			defer func() { _ = tracerProvider.Shutdown(context.Background()) }()
			compiled, err := spec.CompileFile(cmd.Context(), specPath)
			if err != nil {
				return fmt.Errorf("compile spec: %w", err)
			}
			db, err := storage.Open(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("open runtime database: %w", err)
			}
			defer func() { _ = db.Close() }()
			epoch, err := evidence.NewRuntimeEpoch()
			if err != nil {
				return fmt.Errorf("generate runtime epoch: %w", err)
			}
			owner := &storage.RuntimeOwner{DB: db, InstanceID: epoch, Lease: time.Minute}
			ledger := &evidence.Ledger{DB: db, LeaseOwner: epoch, RuntimeEpoch: epoch, Lease: time.Minute}
			epochControl := &storage.EpochControl{DB: db}
			service, err := runtime.NewService(owner, ledger, epoch)
			if err != nil {
				return fmt.Errorf("create runtime service: %w", err)
			}
			if _, err := service.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start runtime: %w", err)
			}
			defer func() { _ = service.Close(context.Background()) }()
			workerRuntime, err := runtime.NewWorkerRuntime(cmd.Context(), runtime.WorkerRuntimeConfig{
				DB: db, Ledger: ledger, RuntimeEpoch: epoch, WorkerSocket: workerSocket, WorkerName: workerName,
				WorkerCA: workerCA, WorkerCert: workerCert, WorkerKey: workerKey, WorkerServerName: workerServerName,
				EvidenceSocket: evidenceSocket, EvidenceKey: evidenceKey, ModelEndpoint: modelEndpoint, ModelName: modelName,
			})
			if err != nil {
				return fmt.Errorf("configure worker runtime: %w", err)
			}
			defer func() { _ = workerRuntime.Close() }()
			effector, serialEffector, closeEffector, err := profileOptions.open(cmd.Context(), db, owner, epochControl, epoch, nil, true)
			if err != nil {
				return fmt.Errorf("configure effect profile: %w", err)
			}
			if closeEffector != nil {
				defer func() { _ = closeEffector() }()
			}
			pipeline, err := runtime.NewPipeline(cmd.Context(), runtime.PipelineConfig{
				DB: db, Spec: compiled, TenantID: tenantID, Owner: owner, OwnerEpoch: epoch,
				Executor: workerRuntime.Executor, Effector: effector, SerialEffector: serialEffector, IDGenerator: ids.Random(),
			})
			if err != nil {
				return fmt.Errorf("create runtime pipeline: %w", err)
			}
			if err := pipeline.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start pipeline maintenance: %w", err)
			}
			defer func() { _ = pipeline.Close() }()
			var report runtime.PipelineReport
			switch traceFormat {
			case "normalized":
				report, err = pipeline.RunJSONL(cmd.Context(), tracePath)
				if err != nil {
					return fmt.Errorf("run normalized trace: %w", err)
				}
			case "simulator":
				report, err = pipeline.RunSimulatorJSONL(cmd.Context(), tracePath)
				if err != nil {
					return fmt.Errorf("run simulator trace: %w", err)
				}
			default:
				return fmt.Errorf("unsupported --trace-format %q", traceFormat)
			}
			if workerErr := readWorkerRuntimeError(workerRuntime); workerErr != nil {
				return workerErr
			}
			cmd.Printf("events_ingested=%d events_processed=%d episodes_admitted=%d episodes_executed=%d intents_evaluated=%d commands_dispatched=%d\n", report.EventsIngested, report.EventsProcessed, report.EpisodesAdmitted, report.EpisodesExecuted, report.IntentsEvaluated, report.CommandsDispatched)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&specPath, "spec", "", "SituationSpec YAML path")
	cmd.Flags().StringVar(&tracePath, "trace", "", "JSONL trace path")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "Tenant ID")
	cmd.Flags().StringVar(&traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	addEffectProfileFlags(cmd, &effectProfile, &deviceSocket, &deviceCatalog, &deviceFirmwareDigests, &liveActuation, &ownerAuthorized)
	cmd.Flags().StringVar(&workerSocket, "worker-socket", "", "EpisodeWorker Unix socket (overrides the native Go executor)")
	cmd.Flags().StringVar(&modelEndpoint, "model-endpoint", "", "OpenAI-compatible model endpoint for the native Go executor")
	cmd.Flags().StringVar(&modelName, "model-name", "", "Model name for the OpenAI-compatible native provider")
	cmd.Flags().StringVar(&workerName, "worker-name", "native", "Expected EpisodeWorker name")
	cmd.Flags().StringVar(&workerCA, "worker-ca", "", "Worker CA PEM (enables mTLS)")
	cmd.Flags().StringVar(&workerCert, "worker-cert", "", "Runtime client certificate PEM")
	cmd.Flags().StringVar(&workerKey, "worker-key", "", "Runtime client private key PEM")
	cmd.Flags().StringVar(&workerServerName, "worker-server-name", "", "Expected worker certificate name")
	cmd.Flags().StringVar(&evidenceSocket, "evidence-socket", "", "Runtime EvidenceTools Unix socket for worker episodes")
	cmd.Flags().StringVar(&evidenceKey, "evidence-key", "", "Hex HMAC key shared with the runtime EvidenceTools verifier")
	return cmd
}

func newServeCommand() *cobra.Command {
	var dbPath, listenAddress, tenantID, specPath, tracePath, liveSocket, traceFormat, effectProfile string
	var modelEndpoint, modelName string
	var workerSocket, workerName, workerCA, workerCert, workerKey, workerServerName, evidenceSocket, evidenceKey string
	var deviceSocket, deviceCatalog string
	var deviceFirmwareDigests []string
	var liveActuation, ownerAuthorized bool
	var ownerLease, pollInterval time.Duration
	var demoMode bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the live Go runtime and readiness endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dbPath == "" {
				return fmt.Errorf("--db is required")
			}
			if err := validateServeSources(specPath, tracePath, liveSocket, workerSocket); err != nil {
				return err
			}
			if liveSocket != "" && traceFormat != "normalized" {
				return fmt.Errorf("--live-socket requires --trace-format normalized")
			}
			profileOptions := effectProfileOptions{
				Profile: actions.EffectProfile(effectProfile), DeviceSocket: deviceSocket, DeviceCatalog: deviceCatalog,
				AllowedFirmwareDigests: deviceFirmwareDigests, LiveActuation: liveActuation,
				OwnerAuthorized: ownerAuthorized,
			}
			if err := profileOptions.validate(tracePath != ""); err != nil {
				return fmt.Errorf("validate effect profile: %w", err)
			}
			workerConfig := runtime.WorkerRuntimeConfig{
				WorkerSocket: workerSocket, WorkerName: workerName, WorkerCA: workerCA, WorkerCert: workerCert,
				WorkerKey: workerKey, WorkerServerName: workerServerName, EvidenceSocket: evidenceSocket, EvidenceKey: evidenceKey,
				ModelEndpoint: modelEndpoint, ModelName: modelName,
			}
			if err := runtime.ValidateWorkerRuntimeConfig(workerConfig); err != nil {
				return err
			}
			if pollInterval <= 0 {
				return fmt.Errorf("--poll-interval must be positive")
			}
			subscriberToken := os.Getenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN")
			if subscriberToken == "" {
				return fmt.Errorf("AGENTIC_STREAM_SUBSCRIBER_TOKEN is required for notification subscribers")
			}
			if !isLoopbackListenAddress(listenAddress) {
				return fmt.Errorf("non-loopback --listen requires an authenticated deployment proxy")
			}
			db, err := storage.Open(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("open runtime database: %w", err)
			}
			defer func() { _ = db.Close() }()
			epoch, err := evidence.NewRuntimeEpoch()
			if err != nil {
				return fmt.Errorf("generate runtime epoch: %w", err)
			}
			owner := &storage.RuntimeOwner{DB: db, InstanceID: epoch, Lease: ownerLease}
			ledger := &evidence.Ledger{DB: db, LeaseOwner: epoch, RuntimeEpoch: epoch, Lease: ownerLease}
			epochControl := &storage.EpochControl{DB: db}
			service, err := runtime.NewService(owner, ledger, epoch)
			if err != nil {
				return fmt.Errorf("create runtime service: %w", err)
			}
			if _, err := service.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start runtime: %w", err)
			}
			defer func() { _ = service.Close(context.Background()) }()
			metrics := telemetry.NewRuntime(time.Now().UTC())
			tracerProvider, telemetryErr := configureRuntimeTelemetry(cmd.Context())
			if telemetryErr != nil {
				return telemetryErr
			}
			defer func() { _ = tracerProvider.Shutdown(context.Background()) }()
			runCtx, stop := context.WithCancel(cmd.Context())
			defer stop()
			effector, serialEffector, closeEffector, err := profileOptions.open(runCtx, db, owner, epochControl, epoch, metrics, tracePath != "")
			if err != nil {
				return fmt.Errorf("configure effect profile: %w", err)
			}
			if closeEffector != nil {
				defer func() { _ = closeEffector() }()
			}
			var pipeline *runtime.Pipeline
			var workerRuntime *runtime.WorkerRuntime
			pipelineErrors := make(chan error, 1)
			if specPath != "" {
				compiled, compileErr := spec.CompileFile(runCtx, specPath)
				if compileErr != nil {
					return fmt.Errorf("compile spec: %w", compileErr)
				}
				workerConfig.DB = db
				workerConfig.Ledger = ledger
				workerConfig.RuntimeEpoch = epoch
				workerRuntime, err = runtime.NewWorkerRuntime(runCtx, workerConfig)
				if err != nil {
					return fmt.Errorf("configure worker runtime: %w", err)
				}
				defer func() { _ = workerRuntime.Close() }()
				if workerRuntime.Errors() != nil {
					go func() {
						select {
						case workerErr := <-workerRuntime.Errors():
							pipelineErrors <- fmt.Errorf("worker runtime: %w", workerErr)
							stop()
						case <-runCtx.Done():
						}
					}()
				}
				pipeline, err = runtime.NewPipeline(runCtx, runtime.PipelineConfig{
					DB: db, Spec: compiled, TenantID: tenantID, Owner: owner, OwnerEpoch: epoch,
					Executor: workerRuntime.Executor, Effector: effector, SerialEffector: serialEffector, IDGenerator: ids.Random(), Telemetry: metrics,
					EpochControl: epochControl, DemoMode: demoMode,
				})
				if err != nil {
					return fmt.Errorf("configure live pipeline: %w", err)
				}
				if err := pipeline.Start(runCtx); err != nil {
					return fmt.Errorf("start live pipeline: %w", err)
				}
				defer func() { _ = pipeline.Close() }()
				if liveSocket != "" {
					go func() {
						if runErr := pipeline.RunLiveSocket(runCtx, liveSocket); runErr != nil && !errors.Is(runErr, context.Canceled) {
							pipelineErrors <- fmt.Errorf("live socket pipeline: %w", runErr)
							stop()
						}
					}()
					if err := waitForLiveSocket(runCtx, liveSocket, pipelineErrors); err != nil {
						return err
					}
				} else {
					go func() {
						for {
							var runErr error
							switch traceFormat {
							case "normalized":
								_, runErr = pipeline.RunJSONL(runCtx, tracePath)
							case "simulator":
								_, runErr = pipeline.RunSimulatorJSONL(runCtx, tracePath)
							default:
								runErr = fmt.Errorf("unsupported --trace-format %q", traceFormat)
							}
							if runErr != nil && !errors.Is(runErr, context.Canceled) {
								pipelineErrors <- fmt.Errorf("continuous pipeline: %w", runErr)
								stop()
								return
							}
							timer := time.NewTimer(pollInterval)
							select {
							case <-runCtx.Done():
								if !timer.Stop() {
									<-timer.C
								}
								return
							case <-timer.C:
							}
						}
					}()
				}
			}
			handler := api.NewRuntimeHandler(service, db, notify.SSEConfig{
				TenantID:  tenantID,
				MaxLag:    1000,
				Authorize: notify.BearerTokenAuthorizer(subscriberToken),
			}, metrics.Handler(), epochControl, epoch, os.Getenv("AGENTIC_STREAM_CONTROL_TOKEN"))
			server := &http.Server{Addr: listenAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
			go func() {
				<-runCtx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
			}()
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("serve runtime: %w", err)
			}
			select {
			case pipelineErr := <-pipelineErrors:
				return pipelineErr
			default:
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&specPath, "spec", "", "SituationSpec YAML path for continuous ingestion")
	cmd.Flags().StringVar(&tracePath, "trace", "", "append-only JSONL trace path for continuous ingestion")
	cmd.Flags().StringVar(&liveSocket, "live-socket", "", "Unix socket for live normalized JSONL telemetry ingestion")
	cmd.Flags().StringVar(&traceFormat, "trace-format", "normalized", "Trace format: normalized or simulator")
	addEffectProfileFlags(cmd, &effectProfile, &deviceSocket, &deviceCatalog, &deviceFirmwareDigests, &liveActuation, &ownerAuthorized)
	cmd.Flags().StringVar(&modelEndpoint, "model-endpoint", "", "OpenAI-compatible model endpoint for the native Go executor")
	cmd.Flags().StringVar(&modelName, "model-name", "", "Model name for the OpenAI-compatible native provider")
	cmd.Flags().StringVar(&workerSocket, "worker-socket", "", "EpisodeWorker Unix socket (overrides the native Go executor)")
	cmd.Flags().StringVar(&workerName, "worker-name", "native", "Expected EpisodeWorker name")
	cmd.Flags().StringVar(&workerCA, "worker-ca", "", "Worker CA PEM (enables mTLS)")
	cmd.Flags().StringVar(&workerCert, "worker-cert", "", "Runtime client certificate PEM")
	cmd.Flags().StringVar(&workerKey, "worker-key", "", "Runtime client private key PEM")
	cmd.Flags().StringVar(&workerServerName, "worker-server-name", "", "Expected worker certificate name")
	cmd.Flags().StringVar(&evidenceSocket, "evidence-socket", "", "Runtime EvidenceTools Unix socket for worker episodes")
	cmd.Flags().StringVar(&evidenceKey, "evidence-key", "", "Hex HMAC key shared with the runtime EvidenceTools verifier")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "tenant served by this runtime process")
	cmd.Flags().StringVar(&listenAddress, "listen", "127.0.0.1:8080", "loopback HTTP listen address")
	cmd.Flags().DurationVar(&ownerLease, "owner-lease", time.Minute, "runtime owner lease duration")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", time.Second, "continuous source polling interval")
	cmd.Flags().BoolVar(&demoMode, "demo-mode", false, "admit fixture executors (demos and tests only; a production route never admits fixture)")
	return cmd
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
	return host == "127.0.0.1" || host == "localhost" || host == "[::1]" || host == "::1"
}

func readWorkerRuntimeError(workerRuntime *runtime.WorkerRuntime) error {
	if workerRuntime == nil || workerRuntime.Errors() == nil {
		return nil
	}
	select {
	case workerErr := <-workerRuntime.Errors():
		return fmt.Errorf("worker runtime: %w", workerErr)
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
