package runtime

import (
	"context"
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
	remoteexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
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
		nativeExecutor, err := newRuntimeNativeExecutor(cfg)
		if err != nil {
			return nil, err
		}
		r.Executor = nativeExecutor
	}
	var evidenceSecret []byte
	if cfg.EvidenceSocket != "" {
		var err error
		if evidenceSecret, err = r.startEvidenceServer(cfg); err != nil {
			return nil, err
		}
	}
	if cfg.WorkerSocket != "" {
		if err := r.connectWorker(ctx, cfg, evidenceSecret); err != nil {
			return nil, err
		}
	}
	cleanupOnError = false
	return r, nil
}

func newRuntimeNativeExecutor(cfg WorkerRuntimeConfig) (episodes.Executor, error) {
	var provider nativeexecutor.ModelProvider = &nativeexecutor.DeterministicProvider{}
	if cfg.ModelEndpoint != "" {
		provider = &nativeexecutor.OpenAICompatibleProvider{Endpoint: cfg.ModelEndpoint, APIKey: os.Getenv("AGENTIC_STREAM_MODEL_API_KEY"), Model: cfg.ModelName}
	}
	nativeExecutor, err := newNativeExecutor(nativeexecutor.Config{
		Provider: provider,
		ToolFactory: func(req *episodes.Request) []nativeexecutor.Tool {
			return []nativeexecutor.Tool{
				nativeexecutor.NewSQLiteEvidenceTool(cfg.DB, "evidence_get", req.TenantID, req.EntityID),
				nativeexecutor.NewSQLiteEvidenceTool(cfg.DB, "evidence.get", req.TenantID, req.EntityID),
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("configure native executor: %w", err)
	}
	return nativeExecutor, nil
}

// startEvidenceServer serves the ledger-backed evidence tools on the private
// evidence socket and returns the capability signing key.
func (r *WorkerRuntime) startEvidenceServer(cfg WorkerRuntimeConfig) ([]byte, error) {
	evidenceSecret, err := decodeEvidenceKey(cfg.EvidenceKey)
	if err != nil {
		return nil, err
	}
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("evidence ledger is required with --evidence-socket")
	}
	listener, err := worker.ListenEvidenceSocket(cfg.EvidenceSocket)
	if err != nil {
		return nil, fmt.Errorf("listen evidence socket: %w", err)
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
	return evidenceSecret, nil
}

// connectWorker dials the EpisodeWorker over its socket, with mTLS when
// configured, and negotiates evidence tools when the evidence server runs.
func (r *WorkerRuntime) connectWorker(ctx context.Context, cfg WorkerRuntimeConfig, evidenceSecret []byte) error {
	tlsConfig, err := loadWorkerTLS(cfg.WorkerCA, cfg.WorkerCert, cfg.WorkerKey, cfg.WorkerServerName)
	if err != nil {
		return err
	}
	conn, err := worker.DialEpisodeWorkerSocketTLS(ctx, cfg.WorkerSocket, tlsConfig)
	if err != nil {
		return fmt.Errorf("dial episode worker socket: %w", err)
	}
	r.workerConn = conn
	client := runtimev1.NewEpisodeWorkerClient(conn)
	if cfg.EvidenceSocket == "" {
		r.Executor = remoteexecutor.NewExecutor(client, cfg.WorkerName, cfg.RuntimeEpoch, nil)
		return nil
	}
	factory := &remoteexecutor.AttemptCapabilityIssuer{
		Issuer: evidenceIssuer(evidenceSecret), RuntimeEpoch: cfg.RuntimeEpoch, Tools: []string{"evidence.get"},
		From: time.Now().UTC().Add(-24 * time.Hour), Until: time.Now().UTC().Add(24 * time.Hour), MaxRows: 1000, MaxBytes: 1 << 20,
	}
	features := []string{worker.EvidenceToolsFeature}
	r.Executor = remoteexecutor.NewExecutorWithEvidence(client, cfg.WorkerName, cfg.RuntimeEpoch, features, cfg.EvidenceSocket, factory)
	return nil
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
