package api

import (
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Readiness reports whether the runtime can safely accept work.
type Readiness = domain.Readiness

// ApprovalSelection identifies a request and the human decision to sign.
type ApprovalSelection = domain.ApprovalSelection

// ApprovalSubmission carries a signed decision; tenant and time belong to runtime.
type ApprovalSubmission = domain.ApprovalSubmission

// ApprovalConfig binds callbacks to one authenticated relay.
type ApprovalConfig = domain.ApprovalConfig

// AuthorizeSubscriber checks the subscriber credential and event type.
type AuthorizeSubscriber = transport.AuthorizeSubscriber

// SSEConfig configures one durable notification stream.
type SSEConfig = transport.SSEConfig

// BearerTokenAuthorizer creates a constant-time subscriber credential check.
func BearerTokenAuthorizer(expected string) AuthorizeSubscriber {
	return transport.BearerTokenAuthorizer(expected)
}

// NewHealthHandler creates /health/live and /health/ready handlers.
func NewHealthHandler(readiness Readiness) http.Handler {
	return transport.NewHealthHandler(readiness)
}

// NewSSEHandler creates a cursor-resumable, at-least-once Server-Sent Events
// handler over the durable notification outbox.
func NewSSEHandler(cfg SSEConfig) http.Handler { return transport.NewSSEHandler(cfg) }

// WithApprovals mounts authenticated approval operations alongside existing routes.
func WithApprovals(base http.Handler, cfg ApprovalConfig) http.Handler {
	return transport.WithApprovals(base, cfg)
}

// NewRuntimeHandler combines health, the notification stream, metrics and the
// epoch controls into the runtime's loopback HTTP surface.
func NewRuntimeHandler(readiness Readiness, db *storage.DB, events SSEConfig, metrics http.Handler, control *runtimecontrol.EpochControl, epoch string, controlToken string) http.Handler {
	return transport.NewRuntimeHandler(readiness, db, events, metrics, control, epoch, controlToken)
}
