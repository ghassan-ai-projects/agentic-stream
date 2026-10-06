// Package transport owns the gRPC calls to one EpisodeWorker: the handshake,
// opening the event stream and translating RPC cancellation into context
// errors. It decides nothing about what a worker may do.
package transport

import (
	"context"
	"fmt"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// EventReceiver is the receive side of the EpisodeWorker Execute stream.
type EventReceiver interface {
	Recv() (*runtimev1.EpisodeEvent, error)
}

// Worker is a connected EpisodeWorker. The connection lifecycle belongs to the
// caller so it can be supervised and shared across episodes.
type Worker struct {
	client runtimev1.EpisodeWorkerClient
}

// NewWorker wraps a connected worker client.
func NewWorker(client runtimev1.EpisodeWorkerClient) Worker { return Worker{client: client} }

// Configured reports whether the worker has a client.
func (w Worker) Configured() bool { return w.client != nil }

// Handshake performs the version and feature exchange.
func (w Worker) Handshake(ctx context.Context, request *runtimev1.HandshakeRequest) (*runtimev1.HandshakeResponse, error) {
	response, err := w.client.Handshake(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("worker handshake: %w", err)
	}
	return response, nil
}

// Open starts one episode and returns its event stream.
func (w Worker) Open(ctx context.Context, request *runtimev1.EpisodeRequest) (EventReceiver, error) {
	stream, err := w.client.Execute(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("execute worker request: %w", err)
	}
	return stream, nil
}
