package remote

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// Executor adapts the streamed EpisodeWorker protocol to the durable
// aggregate Outcome consumed by Runner. It never accepts a Decision before a
// matching terminal stream has been observed.
type Executor struct {
	client                runtimev1.EpisodeWorkerClient
	name                  string
	runtimeInstance       string
	requestedFeatures     []string
	evidenceToolsEndpoint string
	capabilityFactory     CapabilityFactory
}

// CapabilityFactory issues an ephemeral token from the trusted attempt
// request. Implementations must never persist or log the returned bytes.
type CapabilityFactory interface {
	Issue(*episodes.Request) ([]byte, error)
}

var _ episodes.Executor = (*Executor)(nil)

// NewExecutorWithEvidence creates an executor whose evidence capability
// is issued per attempt rather than supplied as caller-controlled bytes.
func NewExecutorWithEvidence(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string, endpoint string, factory CapabilityFactory) *Executor {
	executor := NewExecutor(client, name, runtimeInstance, requestedFeatures)
	executor.evidenceToolsEndpoint = endpoint
	executor.capabilityFactory = factory
	return executor
}

// NewExecutor creates an executor for an already-connected worker. The
// connection lifecycle is owned by the caller so it can be supervised and
// shared across episodes.
func NewExecutor(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string) *Executor {
	return &Executor{
		client: client, name: name, runtimeInstance: runtimeInstance,
		requestedFeatures: append([]string(nil), requestedFeatures...),
	}
}

// Execute performs the current-version handshake and consumes one validated
// server stream. RPC cancellation and deadline statuses also match
// context.Canceled and context.DeadlineExceeded, so the runner classifies them
// as cancellation or timeout without importing the transport.
func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (outcome *episodes.Outcome, err error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("worker client is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("episode request is required")
	}
	return e.executeTraced(ctx, req)
}

func (e *Executor) executeTraced(ctx context.Context, req *episodes.Request) (outcome *episodes.Outcome, err error) {
	executionCtx, span := telemetry.StartSpan(ctx, "agentic_stream.worker.execute")
	telemetry.AddLinkFromW3C(span, req.Traceparent, req.Tracestate)
	span.SetAttributes(attribute.String("agentic_stream.worker", e.name))
	defer func() { finishExecutionSpan(span, err) }()
	outcome, err = e.executeWithinBudget(executionCtx, req)
	return outcome, asContextError(err)
}

func finishExecutionSpan(span trace.Span, err error) {
	if err != nil {
		telemetry.RecordError(span, err)
	}
	span.End()
}

// executeWithinBudget bounds the attempt by its wall-time budget, then
// negotiates, sends the request, and consumes the validated stream.
func (e *Executor) executeWithinBudget(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	executionCtx, cancel, err := boundedExecutionContext(ctx, wallTime)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return e.executeWorker(executionCtx, req)
}

func boundedExecutionContext(ctx context.Context, wallTime time.Duration) (context.Context, context.CancelFunc, error) {
	if wallTime <= 0 {
		return nil, nil, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
	bounded, cancel := context.WithTimeout(ctx, wallTime)
	return bounded, cancel, nil
}

func (e *Executor) executeWorker(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	wireRequest, err := e.wireRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	handshake, err := e.negotiate(ctx, wireRequest)
	if err != nil {
		return nil, err
	}
	return e.consumeWorker(ctx, req, wireRequest, handshake)
}

// wireRequest builds the validated worker request for one attempt, bound to
// the execution deadline and, when evidence tools are enabled, carrying a
// freshly issued capability.
func (e *Executor) wireRequest(ctx context.Context, req *episodes.Request) (*runtimev1.EpisodeRequest, error) {
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
	return e.bindExecutionScope(ctx, req, wireRequest)
}

func (e *Executor) validateEvidenceConfig() error {
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

func (e *Executor) bindExecutionScope(ctx context.Context, req *episodes.Request, wireRequest *runtimev1.EpisodeRequest) (*runtimev1.EpisodeRequest, error) {
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

// negotiate performs the handshake and requires the exact protocol and
// contract versions, the expected worker identity, every requested feature,
// and a request within the worker's size limit.
func (e *Executor) negotiate(ctx context.Context, wireRequest *runtimev1.EpisodeRequest) (*runtimev1.HandshakeResponse, error) {
	handshake, err := e.client.Handshake(ctx, &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: e.name, RuntimeInstanceId: e.runtimeInstance, NonInteractive: true,
		RequestedFeatures: append([]string(nil), e.requestedFeatures...),
	})
	if err != nil {
		return nil, fmt.Errorf("worker handshake: %w", err)
	}
	if err := e.validateHandshake(handshake, wireRequest); err != nil {
		return nil, err
	}
	return handshake, nil
}

func (e *Executor) validateHandshake(handshake *runtimev1.HandshakeResponse, wireRequest *runtimev1.EpisodeRequest) error {
	if handshake.GetProtocolVersion() != worker.ProtocolVersion || handshake.GetContractVersion() != worker.ContractVersion {
		return fmt.Errorf("worker handshake returned unsupported versions")
	}
	if e.name != "" && handshake.GetWorkerName() != e.name {
		return fmt.Errorf("worker handshake identity mismatch")
	}
	return e.validateNegotiatedLimits(handshake, wireRequest)
}

func (e *Executor) validateNegotiatedLimits(handshake *runtimev1.HandshakeResponse, wireRequest *runtimev1.EpisodeRequest) error {
	for _, requested := range e.requestedFeatures {
		if !slices.Contains(handshake.GetSupportedFeatures(), requested) {
			return fmt.Errorf("worker did not negotiate requested feature %q", requested)
		}
	}
	if handshake.GetMaxRequestBytes() > 0 && uint64(proto.Size(wireRequest)) > handshake.GetMaxRequestBytes() { //nolint:gosec // protobuf Size is non-negative and bounded by the negotiated request limit.
		return fmt.Errorf("worker request exceeds negotiated size limit")
	}
	return nil
}

func (e *Executor) consumeWorker(ctx context.Context, req *episodes.Request, wireRequest *runtimev1.EpisodeRequest, handshake *runtimev1.HandshakeResponse) (*episodes.Outcome, error) {
	stream, err := e.client.Execute(ctx, wireRequest)
	if err != nil {
		return nil, fmt.Errorf("execute worker request: %w", err)
	}
	consumer := newWorkerStream(req, wireRequest.GetBudget(), handshake.GetMaxEventBytes())
	if err := consumer.consume(stream); err != nil {
		return nil, err
	}
	return consumer.outcome()
}
