package transport

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AsContextError lets the episode runner classify worker transport
// cancellation and deadline statuses as the matching context errors, without
// importing the transport. Other errors pass through unchanged.
func AsContextError(err error) error {
	switch status.Code(err) {
	case codes.Canceled:
		return contextError{transport: err, cause: context.Canceled}
	case codes.DeadlineExceeded:
		return contextError{transport: err, cause: context.DeadlineExceeded}
	default:
		return err
	}
}

// contextError keeps the transport error's message and gRPC status while also
// matching its context cause under errors.Is.
type contextError struct{ transport, cause error }

func (e contextError) Error() string { return e.transport.Error() }

func (e contextError) Unwrap() []error { return []error{e.transport, e.cause} }
