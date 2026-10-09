package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	nativeexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
	remoteexecutor "github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type WorkerRuntimeConfig struct {
	DB               *storage.DB
	Ledger           *evidence.Service
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

type WorkerBackend struct {
	cfg              WorkerRuntimeConfig
	newNative        NativeConstructor
	workerConn       *grpc.ClientConn
	evidenceGRPC     *grpc.Server
	evidenceListener net.Listener
	evidenceErrors   chan error
}

type NativeConstructor func(nativeexecutor.Config) (*nativeexecutor.Executor, error)

func NewWorkerBackend(cfg WorkerRuntimeConfig, constructor NativeConstructor) *WorkerBackend {
	return &WorkerBackend{cfg: cfg, newNative: constructor, evidenceErrors: make(chan error, 1)}
}

func WorkerOptions(cfg WorkerRuntimeConfig) domain.WorkerOptions {
	return domain.WorkerOptions{
		RuntimeEpoch:     cfg.RuntimeEpoch,
		WorkerSocket:     cfg.WorkerSocket,
		WorkerName:       cfg.WorkerName,
		WorkerCA:         cfg.WorkerCA,
		WorkerCert:       cfg.WorkerCert,
		WorkerKey:        cfg.WorkerKey,
		WorkerServerName: cfg.WorkerServerName,
		EvidenceSocket:   cfg.EvidenceSocket,
		EvidenceKey:      cfg.EvidenceKey,
		ModelEndpoint:    cfg.ModelEndpoint,
		ModelName:        cfg.ModelName,
	}
}

func (r *WorkerBackend) NativeExecutor() (episodes.Executor, error) {
	cfg := r.cfg
	var provider nativeexecutor.ModelProvider = &nativeexecutor.DeterministicProvider{}
	if cfg.ModelEndpoint != "" {
		provider = &nativeexecutor.OpenAICompatibleProvider{Endpoint: cfg.ModelEndpoint, APIKey: os.Getenv("AGENTIC_STREAM_MODEL_API_KEY"), Model: cfg.ModelName}
	}
	nativeExecutor, err := r.newNative(nativeexecutor.Config{
		Provider:    provider,
		ToolFactory: nativeEvidenceTools(cfg.DB),
	})
	if err != nil {
		return nil, fmt.Errorf("configure native executor: %w", err)
	}
	return nativeExecutor, nil
}

func nativeEvidenceTools(db *storage.DB) func(*episodes.Request) []nativeexecutor.Tool {
	return func(req *episodes.Request) []nativeexecutor.Tool {
		horizon, _ := req.EvidenceHorizon()
		return []nativeexecutor.Tool{
			nativeexecutor.NewSQLiteEvidenceTool(db, "evidence_get", req.TenantID, req.EntityID, horizon),
			nativeexecutor.NewSQLiteEvidenceTool(db, "evidence.get", req.TenantID, req.EntityID, horizon),
		}
	}
}

func (r *WorkerBackend) StartEvidence() ([]byte, error) {
	cfg := r.cfg
	evidenceSecret, err := domain.DecodeEvidenceKey(cfg.EvidenceKey)
	if err != nil {
		return nil, err
	}
	listener, err := r.listenEvidence(cfg)
	if err != nil {
		return nil, err
	}
	r.evidenceListener = listener
	if err := r.serveEvidence(cfg, evidenceSecret, listener); err != nil {
		return nil, err
	}
	return evidenceSecret, nil
}

func (r *WorkerBackend) listenEvidence(cfg WorkerRuntimeConfig) (net.Listener, error) {
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("evidence ledger is required with --evidence-socket")
	}
	listener, err := worker.ListenEvidenceSocket(cfg.EvidenceSocket)
	if err != nil {
		return nil, fmt.Errorf("listen evidence socket: %w", err)
	}
	return listener, nil
}

func (r *WorkerBackend) serveEvidence(cfg WorkerRuntimeConfig, evidenceSecret []byte, listener net.Listener) error {
	capabilities, err := evidenceIssuer(evidenceSecret)
	if err != nil {
		return err
	}
	service, err := evidence.New(evidence.Config{Calls: &evidence.CallConfig{Capabilities: capabilities, Ledger: cfg.Ledger, Query: evidence.EventLogQuery(cfg.DB), RuntimeEpoch: cfg.RuntimeEpoch}})
	if err != nil {
		return fmt.Errorf("configure evidence calls: %w", err)
	}
	evidenceGRPC := grpc.NewServer()
	r.evidenceGRPC = evidenceGRPC
	runtimev1.RegisterEvidenceToolsServer(evidenceGRPC, service)
	go r.runEvidenceServer(evidenceGRPC, listener)
	return nil
}

func (r *WorkerBackend) runEvidenceServer(evidenceGRPC *grpc.Server, listener net.Listener) {
	if serveErr := evidenceGRPC.Serve(listener); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
		r.evidenceErrors <- fmt.Errorf("evidence server: %w", serveErr)
	}
}

func (r *WorkerBackend) ConnectWorker(ctx context.Context, evidenceSecret []byte) (episodes.Executor, error) {
	cfg := r.cfg
	tlsConfig, err := loadWorkerTLS(cfg.WorkerCA, cfg.WorkerCert, cfg.WorkerKey, cfg.WorkerServerName)
	if err != nil {
		return nil, err
	}
	conn, err := worker.DialEpisodeWorkerSocketTLS(ctx, cfg.WorkerSocket, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("dial episode worker socket: %w", err)
	}
	r.workerConn = conn
	return r.installRemoteExecutor(cfg, evidenceSecret, conn)
}

func (r *WorkerBackend) installRemoteExecutor(cfg WorkerRuntimeConfig, evidenceSecret []byte, conn *grpc.ClientConn) (episodes.Executor, error) {
	client := runtimev1.NewEpisodeWorkerClient(conn)
	if cfg.EvidenceSocket == "" {
		return remoteexecutor.NewExecutor(client, cfg.WorkerName, cfg.RuntimeEpoch, nil), nil
	}
	issuer, err := evidenceIssuer(evidenceSecret)
	if err != nil {
		return nil, err
	}
	factory := evidenceCapabilityFactory(issuer, cfg.RuntimeEpoch)
	features := []string{worker.EvidenceToolsFeature}
	return remoteexecutor.NewExecutorWithEvidence(client, cfg.WorkerName, cfg.RuntimeEpoch, features, cfg.EvidenceSocket, factory), nil
}

func evidenceCapabilityFactory(issuer *evidence.Service, runtimeEpoch string) *remoteexecutor.AttemptCapabilityIssuer {
	return &remoteexecutor.AttemptCapabilityIssuer{
		Issuer: issuer, RuntimeEpoch: runtimeEpoch, Tools: []string{"evidence.get"},
		Window: evidence.DefaultReadWindow, MaxRows: evidence.DefaultReadMaxRows, MaxBytes: evidence.DefaultReadMaxBytes,
	}
}

func evidenceIssuer(secret []byte) (*evidence.Service, error) {
	service, err := evidence.New(evidence.Config{Capabilities: &evidence.CapabilityConfig{Issuer: "agentic-stream", Audience: "evidence-tools", KeyID: "runtime", Keys: map[string][]byte{"runtime": secret}}})
	if err != nil {
		return nil, fmt.Errorf("configure evidence capabilities: %w", err)
	}
	return service, nil
}

func (r *WorkerBackend) Errors() <-chan error {
	if r == nil || r.evidenceErrors == nil {
		return nil
	}
	return r.evidenceErrors
}

func (r *WorkerBackend) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	errs = r.closeEvidenceServer(errs)
	if r.workerConn != nil {
		if err := r.workerConn.Close(); err != nil {
			errs = append(errs, err)
		}
		r.workerConn = nil
	}
	return errors.Join(errs...)
}

func (r *WorkerBackend) closeEvidenceServer(errs []error) []error {
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
	return errs
}
