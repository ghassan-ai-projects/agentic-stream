// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
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

	return root
}

func newRunLiveCommand() *cobra.Command {
	var dbPath, specPath, tracePath, tenantID, workerSocket, workerName, traceFormat string
	var modelEndpoint, modelName string
	var workerCA, workerCert, workerKey, workerServerName, evidenceSocket, evidenceKey string
	cmd := &cobra.Command{
		Use:   "run-live --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Run one owner-scoped live Go pipeline batch.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if specPath == "" || tracePath == "" || dbPath == "" {
				return fmt.Errorf("--spec, --trace, and --db are required")
			}
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
			service, err := runtime.NewService(owner, ledger, epoch)
			if err != nil {
				return err
			}
			if _, err := service.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start runtime: %w", err)
			}
			defer func() { _ = service.Close(context.Background()) }()
			if evidenceSocket != "" && workerSocket == "" {
				return fmt.Errorf("--evidence-socket requires --worker-socket")
			}
			var evidenceSecret []byte
			var evidenceGRPC *grpc.Server
			var evidenceListener interface{ Close() error }
			if evidenceSocket != "" {
				var decodeErr error
				evidenceSecret, decodeErr = hex.DecodeString(evidenceKey)
				if decodeErr != nil || len(evidenceSecret) < 32 {
					return fmt.Errorf("--evidence-key must be at least 32 bytes of hex")
				}
				listener, listenErr := worker.ListenEvidenceSocket(evidenceSocket)
				if listenErr != nil {
					return fmt.Errorf("listen evidence socket: %w", listenErr)
				}
				evidenceListener = listener
				evidenceGRPC = grpc.NewServer()
				issuer := &evidence.Issuer{Issuer: "agentic-stream", Audience: "evidence-tools", KeyID: "runtime", Keys: map[string][]byte{"runtime": evidenceSecret}}
				runtimev1.RegisterEvidenceToolsServer(evidenceGRPC, &evidence.Server{
					Verifier: &evidence.Verifier{Issuer: issuer.Issuer, Audience: issuer.Audience, Keys: issuer.Keys},
					Query:    makeEvidenceQuery(db), Ledger: ledger, RuntimeEpoch: epoch, RequireLedger: true,
				})
				go func() { _ = evidenceGRPC.Serve(listener) }()
				defer evidenceGRPC.Stop()
				defer func() { _ = evidenceListener.Close() }()
			}

			var provider nativeexecutor.ModelProvider = &nativeexecutor.DeterministicProvider{}
			if modelEndpoint != "" {
				if modelName == "" {
					return fmt.Errorf("--model-name is required with --model-endpoint")
				}
				provider = &nativeexecutor.OpenAICompatibleProvider{Endpoint: modelEndpoint, APIKey: os.Getenv("AGENTIC_STREAM_MODEL_API_KEY"), Model: modelName}
			}
			nativeExecutor, nativeErr := nativeexecutor.New(nativeexecutor.Config{
				Provider: provider,
				ToolFactory: func(req *episodes.Request) []nativeexecutor.Tool {
					return []nativeexecutor.Tool{
						nativeexecutor.NewSQLiteEvidenceTool(db, "evidence_get", req.TenantID, req.EntityID),
						nativeexecutor.NewSQLiteEvidenceTool(db, "evidence.get", req.TenantID, req.EntityID),
					}
				},
			})
			if nativeErr != nil {
				return fmt.Errorf("configure native executor: %w", nativeErr)
			}
			var executor episodes.Executor = nativeExecutor
			var workerConn interface{ Close() error }
			if workerSocket != "" {
				tlsConfig, tlsErr := loadWorkerTLS(workerCA, workerCert, workerKey, workerServerName)
				if tlsErr != nil {
					return tlsErr
				}
				conn, dialErr := worker.DialEpisodeWorkerSocketTLS(cmd.Context(), workerSocket, tlsConfig)
				if dialErr != nil {
					return dialErr
				}
				workerConn = conn
				features := []string(nil)
				if evidenceSocket != "" {
					issuer := &evidence.Issuer{Issuer: "agentic-stream", Audience: "evidence-tools", KeyID: "runtime", Keys: map[string][]byte{"runtime": evidenceSecret}}
					factory := &episodes.AttemptCapabilityIssuer{
						Issuer: issuer, RuntimeEpoch: epoch, Tools: []string{"evidence.get"},
						From: time.Now().UTC().Add(-24 * time.Hour), Until: time.Now().UTC().Add(24 * time.Hour), MaxRows: 1000, MaxBytes: 1 << 20,
					}
					features = []string{worker.EvidenceToolsFeature}
					executor = episodes.NewWorkerExecutorWithEvidence(runtimev1.NewEpisodeWorkerClient(conn), workerName, epoch, features, evidenceSocket, factory)
				} else {
					executor = episodes.NewWorkerExecutor(runtimev1.NewEpisodeWorkerClient(conn), workerName, epoch, features)
				}
			}
			if evidenceSocket != "" && evidenceKey == "" {
				return fmt.Errorf("--evidence-key is required with --evidence-socket")
			}
			if workerConn != nil {
				defer func() { _ = workerConn.Close() }()
			}
			pipeline, err := runtime.NewPipeline(cmd.Context(), runtime.PipelineConfig{
				DB: db, Spec: compiled, TenantID: tenantID, Owner: owner, OwnerEpoch: epoch,
				Executor: executor, Effector: actions.NewSimulatedEffector(), IDGenerator: ids.Random(),
			})
			if err != nil {
				return err
			}
			if err := pipeline.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start pipeline maintenance: %w", err)
			}
			defer func() { _ = pipeline.Close() }()
			var report runtime.PipelineReport
			switch traceFormat {
			case "normalized":
				report, err = pipeline.RunJSONL(cmd.Context(), tracePath)
			case "simulator":
				report, err = pipeline.RunSimulatorJSONL(cmd.Context(), tracePath)
			default:
				return fmt.Errorf("unsupported --trace-format %q", traceFormat)
			}
			if err != nil {
				return err
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

func makeEvidenceQuery(db *storage.DB) evidence.Query {
	return func(ctx context.Context, call evidence.Call) (evidence.QueryResult, error) {
		rows, err := db.QueryContext(ctx, `SELECT event_id, event_type, event_time, payload_json FROM event_log WHERE tenant_id = ? AND entity_id = ? AND event_time >= ? AND event_time <= ? ORDER BY event_time, position LIMIT ?`, call.TenantID, call.EntityID, call.From.UTC().Format(time.RFC3339Nano), call.Until.UTC().Format(time.RFC3339Nano), call.MaxRows)
		if err != nil {
			return evidence.QueryResult{}, fmt.Errorf("query evidence events: %w", err)
		}
		defer func() { _ = rows.Close() }()
		resultRows := make([]map[string]any, 0)
		for rows.Next() {
			var eventID, eventType, eventTime string
			var payload []byte
			if err := rows.Scan(&eventID, &eventType, &eventTime, &payload); err != nil {
				return evidence.QueryResult{}, fmt.Errorf("scan evidence event: %w", err)
			}
			var data map[string]any
			if err := json.Unmarshal(payload, &data); err != nil {
				return evidence.QueryResult{}, fmt.Errorf("decode evidence payload: %w", err)
			}
			resultRows = append(resultRows, map[string]any{"event_id": eventID, "event_type": eventType, "event_time": eventTime, "data": data})
		}
		if err := rows.Err(); err != nil {
			return evidence.QueryResult{}, fmt.Errorf("iterate evidence events: %w", err)
		}
		result, err := json.Marshal(map[string]any{"rows": resultRows})
		if err != nil {
			return evidence.QueryResult{}, fmt.Errorf("encode evidence result: %w", err)
		}
		return evidence.QueryResult{JSON: result, RowCount: uint64(len(resultRows))}, nil //nolint:gosec // Result rows are bounded by the authenticated capability.
	}
}

func loadWorkerTLS(caPath, certPath, keyPath, serverName string) (*tls.Config, error) {
	if caPath == "" && certPath == "" && keyPath == "" {
		return nil, nil
	}
	if caPath == "" || certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("--worker-ca, --worker-cert, and --worker-key are required together")
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read worker CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("worker CA contains no certificates")
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load worker client certificate: %w", err)
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13, ServerName: serverName}, nil
}

func newServeCommand() *cobra.Command {
	var dbPath, listenAddress, tenantID string
	var ownerLease time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the live Go runtime and readiness endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dbPath == "" {
				return fmt.Errorf("--db is required")
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
			service, err := runtime.NewService(owner, ledger, epoch)
			if err != nil {
				return err
			}
			if _, err := service.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start runtime: %w", err)
			}
			defer func() { _ = service.Close(context.Background()) }()
			metrics := telemetry.NewRuntime(time.Now().UTC())
			subscriberToken := os.Getenv("AGENTIC_STREAM_SUBSCRIBER_TOKEN")
			if subscriberToken == "" {
				return fmt.Errorf("AGENTIC_STREAM_SUBSCRIBER_TOKEN is required for notification subscribers")
			}
			if !isLoopbackListenAddress(listenAddress) {
				return fmt.Errorf("non-loopback --listen requires an authenticated deployment proxy")
			}
			handler := api.NewRuntimeHandler(service, db, notify.SSEConfig{
				TenantID:          tenantID,
				MaxLag:            1000,
				Authorize:         notify.BearerTokenAuthorizer(subscriberToken),
			}, metrics.Handler())
			server := &http.Server{Addr: listenAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
			go func() {
				<-cmd.Context().Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
			}()
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("serve runtime: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "tenant served by this runtime process")
	cmd.Flags().StringVar(&listenAddress, "listen", "127.0.0.1:8080", "loopback HTTP listen address")
	cmd.Flags().DurationVar(&ownerLease, "owner-lease", time.Minute, "runtime owner lease duration")
	return cmd
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "localhost" || host == "[::1]" || host == "::1"
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
