package episodes

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WorkerExecutor adapts the streamed EpisodeWorker protocol to the durable
// aggregate Outcome consumed by Runner. It never accepts a Decision before a
// matching terminal stream has been observed.
type WorkerExecutor struct {
	client                runtimev1.EpisodeWorkerClient
	name                  string
	runtimeInstance       string
	requestedFeatures     []string
	evidenceToolsEndpoint string
	capabilityFactory     CapabilityFactory
}

type budgetExceededError struct{ metric string }

func (e *budgetExceededError) Error() string { return "episode budget exceeded: " + e.metric }

type budgetTelemetryMissingError struct{}

func (budgetTelemetryMissingError) Error() string { return "episode budget telemetry is missing" }

// CapabilityFactory issues an ephemeral token from the trusted attempt
// request. Implementations must never persist or log the returned bytes.
type CapabilityFactory interface {
	Issue(*Request) ([]byte, error)
}

// NewWorkerExecutor creates an executor for an already-connected worker. The
// connection lifecycle is owned by the caller so it can be supervised and
// shared across episodes.
func NewWorkerExecutor(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string) *WorkerExecutor {
	return &WorkerExecutor{
		client: client, name: name, runtimeInstance: runtimeInstance,
		requestedFeatures: append([]string(nil), requestedFeatures...),
	}
}

// NewWorkerExecutorWithEvidence creates an executor whose evidence capability
// is issued per attempt rather than supplied as caller-controlled bytes.
func NewWorkerExecutorWithEvidence(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string, endpoint string, factory CapabilityFactory) *WorkerExecutor {
	executor := NewWorkerExecutor(client, name, runtimeInstance, requestedFeatures)
	executor.evidenceToolsEndpoint = endpoint
	executor.capabilityFactory = factory
	return executor
}

// Execute performs the current-version handshake and consumes one validated
// server stream. RPC cancellation and deadline errors are wrapped with %w so
// the caller can still classify them as cancellation or timeout.
func (e *WorkerExecutor) Execute(ctx context.Context, req *Request) (outcome *Outcome, err error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("worker client is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("episode request is required")
	}
	executionCtx, span := telemetry.StartSpan(ctx, "agentic_stream.worker.execute")
	telemetry.AddLinkFromW3C(span, req.Traceparent, req.Tracestate)
	span.SetAttributes(attribute.String("agentic_stream.worker", e.name))
	defer func() {
		if err != nil {
			telemetry.RecordError(span, err)
		}
		span.End()
	}()
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	executionCtx, cancel, err := boundedExecutionContext(executionCtx, wallTime)
	if err != nil {
		return nil, err
	}
	defer cancel()
	wireRequest, err := e.wireRequest(executionCtx, req)
	if err != nil {
		return nil, err
	}
	handshake, err := e.negotiate(executionCtx, wireRequest)
	if err != nil {
		return nil, err
	}
	stream, err := e.client.Execute(executionCtx, wireRequest)
	if err != nil {
		return nil, fmt.Errorf("execute worker request: %w", err)
	}
	consumer := newWorkerStream(req, wireRequest.GetBudget(), handshake.GetMaxEventBytes())
	if err := consumer.consume(stream); err != nil {
		return nil, err
	}
	return consumer.outcome()
}

// wireRequest builds the validated worker request for one attempt, bound to
// the execution deadline and, when evidence tools are enabled, carrying a
// freshly issued capability.
func (e *WorkerExecutor) wireRequest(ctx context.Context, req *Request) (*runtimev1.EpisodeRequest, error) {
	if err := e.validateEvidenceConfig(); err != nil {
		return nil, err
	}
	wireRequest, err := episodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build worker request: %w", err)
	}
	if err := worker.ValidateBudget(wireRequest.GetBudget()); err != nil {
		return nil, fmt.Errorf("worker request budget: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		wireRequest.Deadline = timestamppb.New(deadline.UTC())
	}
	wireRequest.EvidenceToolsEndpoint = e.evidenceToolsEndpoint
	if e.evidenceToolsEndpoint != "" {
		capabilityToken, err := e.capabilityFactory.Issue(req)
		if err != nil {
			return nil, fmt.Errorf("issue evidence capability: %w", err)
		}
		wireRequest.CapabilityToken = append([]byte(nil), capabilityToken...)
	}
	return wireRequest, nil
}

func (e *WorkerExecutor) validateEvidenceConfig() error {
	if e.evidenceToolsEndpoint == "" {
		return nil
	}
	if err := worker.ValidateEvidenceSocketPath(e.evidenceToolsEndpoint); err != nil {
		return fmt.Errorf("evidence endpoint: %w", err)
	}
	if !slices.Contains(e.requestedFeatures, worker.EvidenceToolsFeature) {
		return fmt.Errorf("evidence tools require negotiated feature %q", worker.EvidenceToolsFeature)
	}
	if e.capabilityFactory == nil {
		return fmt.Errorf("evidence capability factory is not configured")
	}
	return nil
}

// negotiate performs the handshake and requires the exact protocol and
// contract versions, the expected worker identity, every requested feature,
// and a request within the worker's size limit.
func (e *WorkerExecutor) negotiate(ctx context.Context, wireRequest *runtimev1.EpisodeRequest) (*runtimev1.HandshakeResponse, error) {
	handshake, err := e.client.Handshake(ctx, &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: e.name, RuntimeInstanceId: e.runtimeInstance, NonInteractive: true,
		RequestedFeatures: append([]string(nil), e.requestedFeatures...),
	})
	if err != nil {
		return nil, fmt.Errorf("worker handshake: %w", err)
	}
	if handshake.GetProtocolVersion() != worker.ProtocolVersion || handshake.GetContractVersion() != worker.ContractVersion {
		return nil, fmt.Errorf("worker handshake returned unsupported versions")
	}
	if e.name != "" && handshake.GetWorkerName() != e.name {
		return nil, fmt.Errorf("worker handshake identity mismatch")
	}
	for _, requested := range e.requestedFeatures {
		if !slices.Contains(handshake.GetSupportedFeatures(), requested) {
			return nil, fmt.Errorf("worker did not negotiate requested feature %q", requested)
		}
	}
	if handshake.GetMaxRequestBytes() > 0 && uint64(proto.Size(wireRequest)) > handshake.GetMaxRequestBytes() { //nolint:gosec // protobuf Size is non-negative and bounded by the negotiated request limit.
		return nil, fmt.Errorf("worker request exceeds negotiated size limit")
	}
	return handshake, nil
}

func boundedExecutionContext(ctx context.Context, wallTime time.Duration) (context.Context, context.CancelFunc, error) {
	if wallTime <= 0 {
		return nil, nil, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
	bounded, cancel := context.WithTimeout(ctx, wallTime)
	return bounded, cancel, nil
}

var _ Executor = (*WorkerExecutor)(nil)
