package remote

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote/internal/app"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// CapabilityFactory issues an ephemeral token from the trusted attempt request.
// Implementations must never persist or log the returned bytes.
type CapabilityFactory = app.CapabilityFactory

// AttemptCapabilityIssuer derives one short-lived evidence capability from a
// trusted durable Request. The raw token exists only in the dispatch call.
type AttemptCapabilityIssuer = app.AttemptCapabilityIssuer

// Executor adapts the streamed EpisodeWorker protocol to the durable aggregate
// Outcome consumed by Runner.
type Executor struct {
	app *app.Executor
}

var _ episodes.Executor = (*Executor)(nil)

// NewExecutor creates an executor for an already-connected worker. The
// connection lifecycle is owned by the caller.
func NewExecutor(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string) *Executor {
	return &Executor{app: app.New(client, name, runtimeInstance, requestedFeatures)}
}

// NewExecutorWithEvidence creates an executor whose evidence capability is
// issued per attempt rather than supplied as caller-controlled bytes.
func NewExecutorWithEvidence(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string, endpoint string, factory CapabilityFactory) *Executor {
	return &Executor{app: app.NewWithEvidence(client, name, runtimeInstance, requestedFeatures, endpoint, factory)}
}

// Execute runs one episode on the worker and returns its validated Outcome.
func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	return e.app.Execute(ctx, req) //nolint:wrapcheck // The application names each failed step.
}
