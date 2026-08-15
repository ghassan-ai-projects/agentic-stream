package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// newNativeExecutor is the native-executor constructor, injectable so the
// P1 gate-8 adversarial test can prove a tamoz route never constructs it.
var newNativeExecutor = nativeexecutor.New

// WorkerRuntimeConfig contains the complete validated composition for native
// or process-isolated episode execution. The same configuration is used by
// run-live and serve so their worker and evidence boundaries cannot drift.
type WorkerRuntimeConfig struct {
	DB               *storage.DB
	Ledger           *evidence.Ledger
	RuntimeEpoch     string
	WorkerSocket     string
	WorkerName       string
	WorkerCA         string
	WorkerCert       string
	WorkerKey        string
	WorkerServerName string
	EvidenceSocket   string
	EvidenceKey      string
	ModelEndpoint    string
	ModelName        string
}

// WorkerRuntime owns the executor connection and the runtime-side evidence
// gRPC server. Close is safe to call on partially initialized instances.
type WorkerRuntime struct {
	Executor         episodes.Executor
	workerConn       *grpc.ClientConn
	evidenceGRPC     *grpc.Server
	evidenceListener net.Listener
	evidenceErrors   chan error
}

// ValidateWorkerRuntimeConfig rejects incomplete worker/evidence combinations
// before sockets or credentials are opened.
func ValidateWorkerRuntimeConfig(cfg WorkerRuntimeConfig) error {
	if cfg.EvidenceSocket != "" && cfg.WorkerSocket == "" {
		return fmt.Errorf("--evidence-socket requires --worker-socket")
	}
	if cfg.EvidenceKey != "" && cfg.EvidenceSocket == "" {
		return fmt.Errorf("--evidence-key requires --evidence-socket")
	}
	if cfg.EvidenceSocket != "" {
		secret, err := hex.DecodeString(cfg.EvidenceKey)
		if err != nil || len(secret) < 32 {
			return fmt.Errorf("--evidence-key must be at least 32 bytes of hex")
		}
	}
	if cfg.WorkerSocket != "" && cfg.WorkerName == "" {
		return fmt.Errorf("--worker-name is required with --worker-socket")
	}
	if cfg.WorkerSocket == "" && (cfg.WorkerCA != "" || cfg.WorkerCert != "" || cfg.WorkerKey != "" || cfg.WorkerServerName != "") {
		return fmt.Errorf("worker TLS flags require --worker-socket")
	}
	if (cfg.WorkerCA != "") != (cfg.WorkerCert != "") || (cfg.WorkerCA != "") != (cfg.WorkerKey != "") {
		return fmt.Errorf("--worker-ca, --worker-cert, and --worker-key are required together")
	}
	if cfg.WorkerCA == "" && cfg.WorkerServerName != "" {
		return fmt.Errorf("--worker-server-name requires mTLS")
	}
	if cfg.WorkerCA != "" && cfg.WorkerServerName == "" {
		return fmt.Errorf("--worker-server-name is required with mTLS")
	}
	if cfg.ModelEndpoint != "" && cfg.ModelName == "" {
		return fmt.Errorf("--model-name is required with --model-endpoint")
	}
	return nil
}

// NewWorkerRuntime constructs the native executor by default and replaces it
// with the authenticated EpisodeWorker client when WorkerSocket is configured.
// Evidence tools, when configured, are started before the worker handshake.
func NewWorkerRuntime(ctx context.Context, cfg WorkerRuntimeConfig) (*WorkerRuntime, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("worker runtime database is required")
	}
	if err := ValidateWorkerRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = "native"
	}
	r := &WorkerRuntime{evidenceErrors: make(chan error, 1)}
	cleanupOnError := true
	var err error
	defer func() {
		if cleanupOnError {
			_ = r.Close()
		}
	}()

	// P1 gate 8 (B1): the native executor is NEVER constructed on a tamoz
	// route. A configured worker socket IS the tamoz route (the Go runtime
	// delegates the episode to the out-of-process Ruby worker); constructing
	// the native executor here would violate "on an ExecutorName=tamoz route
	// the Go native executor is never constructed". Native mode keeps the
	// constructor.
	if cfg.WorkerSocket == "" {
		var provider nativeexecutor.ModelProvider = &nativeexecutor.DeterministicProvider{}
		if cfg.ModelEndpoint != "" {
			provider = &nativeexecutor.OpenAICompatibleProvider{Endpoint: cfg.ModelEndpoint, APIKey: os.Getenv("AGENTIC_STREAM_MODEL_API_KEY"), Model: cfg.ModelName}
		}
		nativeExecutor, newErr := newNativeExecutor(nativeexecutor.Config{
			Provider: provider,
			ToolFactory: func(req *episodes.Request) []nativeexecutor.Tool {
				return []nativeexecutor.Tool{
					nativeexecutor.NewSQLiteEvidenceTool(cfg.DB, "evidence_get", req.TenantID, req.EntityID),
					nativeexecutor.NewSQLiteEvidenceTool(cfg.DB, "evidence.get", req.TenantID, req.EntityID),
				}
			},
		})
		if newErr != nil {
			return nil, fmt.Errorf("configure native executor: %w", newErr)
		}
		r.Executor = nativeExecutor
	}

	var evidenceSecret []byte
	if cfg.EvidenceSocket != "" {
		evidenceSecret, err = hex.DecodeString(cfg.EvidenceKey)
		if err != nil {
			return nil, fmt.Errorf("decode evidence key: %w", err)
		}
		if len(evidenceSecret) < 32 {
			return nil, fmt.Errorf("--evidence-key must be at least 32 bytes of hex")
		}
		if cfg.Ledger == nil {
			return nil, fmt.Errorf("evidence ledger is required with --evidence-socket")
		}
		listener, listenErr := worker.ListenEvidenceSocket(cfg.EvidenceSocket)
		if listenErr != nil {
			return nil, fmt.Errorf("listen evidence socket: %w", listenErr)
		}
		r.evidenceListener = listener
		evidenceGRPC := grpc.NewServer()
		r.evidenceGRPC = evidenceGRPC
		issuer := evidenceIssuer(evidenceSecret)
		runtimev1.RegisterEvidenceToolsServer(evidenceGRPC, &evidence.Server{
			Verifier: &evidence.Verifier{Issuer: issuer.Issuer, Audience: issuer.Audience, Keys: issuer.Keys},
			Query:    makeEvidenceQuery(cfg.DB), Ledger: cfg.Ledger, RuntimeEpoch: cfg.RuntimeEpoch, RequireLedger: true,
		})
		go func() {
			if serveErr := evidenceGRPC.Serve(listener); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
				r.evidenceErrors <- fmt.Errorf("evidence server: %w", serveErr)
			}
		}()
	}

	if cfg.WorkerSocket != "" {
		tlsConfig, tlsErr := loadWorkerTLS(cfg.WorkerCA, cfg.WorkerCert, cfg.WorkerKey, cfg.WorkerServerName)
		if tlsErr != nil {
			return nil, tlsErr
		}
		conn, dialErr := worker.DialEpisodeWorkerSocketTLS(ctx, cfg.WorkerSocket, tlsConfig)
		if dialErr != nil {
			return nil, fmt.Errorf("dial episode worker socket: %w", dialErr)
		}
		r.workerConn = conn
		features := []string(nil)
		if cfg.EvidenceSocket != "" {
			issuer := evidenceIssuer(evidenceSecret)
			factory := &episodes.AttemptCapabilityIssuer{
				Issuer: issuer, RuntimeEpoch: cfg.RuntimeEpoch, Tools: []string{"evidence.get"},
				From: time.Now().UTC().Add(-24 * time.Hour), Until: time.Now().UTC().Add(24 * time.Hour), MaxRows: 1000, MaxBytes: 1 << 20,
			}
			features = []string{worker.EvidenceToolsFeature}
			r.Executor = episodes.NewWorkerExecutorWithEvidence(runtimev1.NewEpisodeWorkerClient(conn), cfg.WorkerName, cfg.RuntimeEpoch, features, cfg.EvidenceSocket, factory)
		} else {
			r.Executor = episodes.NewWorkerExecutor(runtimev1.NewEpisodeWorkerClient(conn), cfg.WorkerName, cfg.RuntimeEpoch, features)
		}
	}
	cleanupOnError = false
	return r, nil
}

// Errors reports asynchronous evidence-server failures. A closed channel is
// not used: callers select it alongside their process lifecycle context.
func (r *WorkerRuntime) Errors() <-chan error {
	if r == nil || r.evidenceErrors == nil {
		return nil
	}
	return r.evidenceErrors
}

// Close stops the evidence server and closes the worker connection.
func (r *WorkerRuntime) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.evidenceGRPC != nil {
		r.evidenceGRPC.Stop()
		r.evidenceGRPC = nil
	}
	if r.evidenceListener != nil {
		if err := r.evidenceListener.Close(); err != nil {
			errs = append(errs, err)
		}
		r.evidenceListener = nil
	}
	if r.workerConn != nil {
		if err := r.workerConn.Close(); err != nil {
			errs = append(errs, err)
		}
		r.workerConn = nil
	}
	return errors.Join(errs...)
}

func evidenceIssuer(secret []byte) *evidence.Issuer {
	return &evidence.Issuer{Issuer: "agentic-stream", Audience: "evidence-tools", KeyID: "runtime", Keys: map[string][]byte{"runtime": secret}}
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
