// Package app runs one episode on an out-of-process worker: it bounds the
// attempt by its wall-time budget, builds and capability-binds the wire
// request, negotiates the handshake, and consumes the validated event stream
// into the episode Outcome. It decides through domain and talks to the worker
// through transport.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type CapabilityFactory interface {
	Issue(*episodes.Request) (EvidenceGrant, error)
}

type Executor struct {
	worker                transport.Worker
	profile               domain.Profile
	evidenceToolsEndpoint string
	capabilityFactory     CapabilityFactory
}

var _ episodes.Executor = (*Executor)(nil)

func New(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string) *Executor {
	return &Executor{
		worker:  transport.NewWorker(client),
		profile: domain.Profile{Name: name, RuntimeInstance: runtimeInstance, RequestedFeatures: slices.Clone(requestedFeatures)},
	}
}

func NewWithEvidence(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string, endpoint string, factory CapabilityFactory) *Executor {
	executor := New(client, name, runtimeInstance, requestedFeatures)
	executor.evidenceToolsEndpoint = endpoint
	executor.capabilityFactory = factory
	return executor
}

func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	if e == nil || !e.worker.Configured() {
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
	span.SetAttributes(attribute.String("agentic_stream.worker", e.profile.Name))
	defer func() { finishExecutionSpan(span, err) }()
	outcome, err = e.executeWithinBudget(executionCtx, req)
	return outcome, transport.AsContextError(err)
}

func finishExecutionSpan(span trace.Span, err error) {
	if err != nil {
		telemetry.RecordError(span, err)
	}
	span.End()
}

func (e *Executor) executeWithinBudget(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	if wallTime <= 0 {
		return nil, fmt.Errorf("invalid wall_time budget %q", wallTime)
	}
	bounded, cancel := context.WithTimeout(ctx, wallTime)
	defer cancel()
	return e.executeWorker(bounded, req)
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

func (e *Executor) wireRequest(ctx context.Context, req *episodes.Request) (*runtimev1.EpisodeRequest, error) {
	if err := e.profile.ValidateEvidenceConfig(e.evidenceToolsEndpoint, e.capabilityFactory != nil); err != nil {
		return nil, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
	}
	wireRequest, err := domain.WireRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build worker request: %w", err)
	}
	if err := worker.ValidateBudget(wireRequest.GetBudget()); err != nil {
		return nil, fmt.Errorf("worker request budget: %w", err)
	}
	return e.bindExecutionScope(ctx, req, wireRequest)
}

func (e *Executor) bindExecutionScope(ctx context.Context, req *episodes.Request, wireRequest *runtimev1.EpisodeRequest) (*runtimev1.EpisodeRequest, error) {
	if deadline, ok := ctx.Deadline(); ok {
		wireRequest.Deadline = timestamppb.New(deadline.UTC())
	}
	wireRequest.EvidenceToolsEndpoint = e.evidenceToolsEndpoint
	if e.evidenceToolsEndpoint != "" {
		grant, err := e.capabilityFactory.Issue(req)
		if err != nil {
			return nil, fmt.Errorf("issue evidence capability: %w", err)
		}
		wireRequest.CapabilityToken = slices.Clone(grant.Token)
		wireRequest.EvidenceTimeRange = &runtimev1.EvidenceTimeRange{From: timestamppb.New(grant.From), Until: timestamppb.New(grant.Until)}
	}
	return wireRequest, nil
}

func (e *Executor) negotiate(ctx context.Context, wireRequest *runtimev1.EpisodeRequest) (*runtimev1.HandshakeResponse, error) {
	handshake, err := e.worker.Handshake(ctx, e.profile.HandshakeRequest())
	if err != nil {
		return nil, err //nolint:wrapcheck // The transport names the failed call.
	}
	if err := e.profile.ValidateHandshake(handshake, wireRequest); err != nil {
		return nil, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
	}
	return handshake, nil
}

func (e *Executor) consumeWorker(ctx context.Context, req *episodes.Request, wireRequest *runtimev1.EpisodeRequest, handshake *runtimev1.HandshakeResponse) (*episodes.Outcome, error) {
	events, err := e.worker.Open(ctx, wireRequest)
	if err != nil {
		return nil, err //nolint:wrapcheck // The transport names the failed call.
	}
	stream := domain.NewStream(req, wireRequest.GetBudget(), handshake.GetMaxEventBytes())
	if err := consume(events, stream); err != nil {
		return nil, err
	}
	return stream.Outcome() //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

func consume(events transport.EventReceiver, stream *domain.Stream) error {
	for {
		event, err := events.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive worker event: %w", err)
		}
		if err := stream.Accept(event); err != nil {
			return err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
		}
	}
}
