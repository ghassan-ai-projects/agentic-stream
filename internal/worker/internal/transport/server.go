// Package transport serves and dials the episode worker protocol over gRPC: the
// reference worker Server, the private Unix sockets (evidence and worker) and the
// per-stream send guard. The protocol rules live in domain.
package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker/internal/domain"
)

// ExecuteFunc emits worker-originated events after the server has emitted the
// initial EpisodeStarted event. The function must emit exactly one Terminal
// event. The runtime validates every emitted event before it reaches the wire.
type ExecuteFunc func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error

// Server is a validating EpisodeWorker implementation. It deliberately has no
// access to effectors, credentials, persistence, or unbounded tool handles.
type Server struct {
	runtimev1.UnimplementedEpisodeWorkerServer

	WorkerName        string
	WorkerVersion     string
	SupportedFeatures []string
	MaxRequestBytes   uint64
	MaxEventBytes     uint64
	MaxEvents         uint64
	MaxStreamBytes    uint64
	Now               func() time.Time
	ExecuteFunc       ExecuteFunc
}

func (s *Server) limits() domain.Limits {
	return domain.Limits{MaxRequestBytes: s.MaxRequestBytes, MaxEventBytes: s.MaxEventBytes, MaxEvents: s.MaxEvents, MaxStreamBytes: s.MaxStreamBytes}.Resolved()
}

// Handshake accepts only the current protocol and contract versions and
// rejects required features that this worker does not advertise.
func (s *Server) Handshake(_ context.Context, req *runtimev1.HandshakeRequest) (*runtimev1.HandshakeResponse, error) { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	if err := domain.ValidateHandshake(req); err != nil {
		return nil, err //nolint:wrapcheck // gRPC status errors are the public wire contract.
	}
	if err := domain.RequireFeatures(s.SupportedFeatures, req.GetRequestedFeatures()); err != nil {
		return nil, err //nolint:wrapcheck // gRPC status errors are the public wire contract.
	}
	return domain.NewHandshakeResponse(s.WorkerName, s.WorkerVersion, s.SupportedFeatures, s.limits()), nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// guardedStream serializes the sends of one episode stream and validates every
// event before it reaches the wire.
type guardedStream struct {
	mu        sync.Mutex
	validator *domain.StreamValidator
	stream    runtimev1.EpisodeWorker_ExecuteServer
}

func (g *guardedStream) emit(event *runtimev1.EpisodeEvent) error { //nolint:wrapcheck // gRPC status errors are the public wire contract.
	g.mu.Lock()
	defer g.mu.Unlock()
	eventBytes, err := g.validator.Check(event)
	if err != nil {
		return err
	}
	g.validator.Record(event, eventBytes)
	if err := g.stream.Send(event); err != nil {
		return fmt.Errorf("send episode event: %w", err)
	}
	return nil
}
