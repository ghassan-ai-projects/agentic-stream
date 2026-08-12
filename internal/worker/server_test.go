package worker

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestServerHandshakeAndExecute(t *testing.T) {
	srv := newTestServer()
	conn := newBufConn(t, srv)
	client := runtimev1.NewEpisodeWorkerClient(conn)

	handshake, err := client.Handshake(t.Context(), &runtimev1.HandshakeRequest{
		ProtocolVersion: "1.0", ContractVersion: "1.0", WorkerId: "worker-1", RuntimeInstanceId: "runtime-1",
		RequestedFeatures: []string{"trace_context"}, NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if handshake.GetProtocolVersion() != ProtocolVersion || handshake.GetMaxEventBytes() != DefaultMaxEventBytes {
		t.Fatalf("unexpected handshake response: %v", handshake)
	}

	stream, err := client.Execute(t.Context(), validRequest(), grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var events []*runtimev1.EpisodeEvent
	for {
		event, recvErr := stream.Recv()
		if recvErr != nil {
			if !errors.Is(recvErr, io.EOF) {
				if status.Code(recvErr) != codes.OK {
					t.Fatalf("receive event: %v", recvErr)
				}
			}
			break
		}
		events = append(events, event)
	}
	if len(events) != 3 || events[0].GetStarted() == nil || events[1].GetDecision() == nil || events[2].GetTerminal() == nil {
		t.Fatalf("unexpected event stream: %v", events)
	}
}

func TestServerRejectsUnsupportedHandshakeAndTrace(t *testing.T) {
	client := runtimev1.NewEpisodeWorkerClient(newBufConn(t, newTestServer()))
	_, err := client.Handshake(t.Context(), &runtimev1.HandshakeRequest{
		ProtocolVersion: "2.0", ContractVersion: "1.0", WorkerId: "worker-1", RuntimeInstanceId: "runtime-1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("protocol mismatch code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
	_, err = client.Handshake(t.Context(), &runtimev1.HandshakeRequest{
		ProtocolVersion: "1.1", ContractVersion: "1.0", WorkerId: "worker-1", RuntimeInstanceId: "runtime-1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("minor protocol mismatch code = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}

	stream, err := client.Execute(t.Context(), func() *runtimev1.EpisodeRequest {
		request := validRequest()
		request.Traceparent = "not-a-trace"
		return request
	}())
	if err != nil {
		t.Fatalf("trace execute: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("trace mismatch code = %v, want %v", status.Code(err), codes.InvalidArgument)
	}
}

func TestServerRejectsEventSequenceAndMissingTerminal(t *testing.T) {
	srv := newTestServer()
	srv.ExecuteFunc = func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 4, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload:    &runtimev1.EpisodeEvent_Diagnostic{Diagnostic: &runtimev1.Diagnostic{Code: "bad_sequence"}},
		})
	}
	client := runtimev1.NewEpisodeWorkerClient(newBufConn(t, srv))
	stream, err := client.Execute(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for {
		_, recvErr := stream.Recv()
		if recvErr != nil {
			if status.Code(recvErr) != codes.FailedPrecondition {
				t.Fatalf("sequence error = %v, want FailedPrecondition", recvErr)
			}
			break
		}
	}

	srv.ExecuteFunc = func(_ context.Context, _ *runtimev1.EpisodeRequest, _ func(*runtimev1.EpisodeEvent) error) error {
		return nil
	}
	stream, err = client.Execute(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("execute without terminal: %v", err)
	}
	for {
		_, recvErr := stream.Recv()
		if recvErr != nil {
			if status.Code(recvErr) != codes.FailedPrecondition {
				t.Fatalf("missing terminal error = %v, want FailedPrecondition", recvErr)
			}
			break
		}
	}
}

func newTestServer() *Server {
	server := &Server{
		WorkerName: "test-worker", WorkerVersion: "test-v1", SupportedFeatures: []string{"trace_context"},
		Now: func() time.Time { return time.Unix(10, 0).UTC() },
	}
	server.ExecuteFunc = func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		if err := emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
				DecisionJson: []byte(`{"decision_id":"decision-1"}`), DecisionSha256: make([]byte, 32),
				EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			}},
		}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload:    &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED}},
		})
	}
	return server
}

func validRequest() *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{
		ProtocolVersion: "1.0", EpisodeId: "episode-1", TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1,
		SnapshotJson: []byte(`{"situation_id":"situation-1"}`), SnapshotSha256: make([]byte, 32),
		DecisionSchemaJson: []byte(`{"type":"object"}`), ToolCatalogJson: []byte(`[]`), SpecSha256: make([]byte, 32),
		Kind: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE, Lane: runtimev1.EpisodeLane_EPISODE_LANE_FAST,
		RiskCeiling: runtimev1.RiskClass_RISK_CLASS_R1, AttemptId: "attempt-1", Fence: 1,
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Deadline:    timestamppb.New(time.Now().Add(time.Minute)),
	}
}

func newBufConn(t *testing.T, server runtimev1.EpisodeWorkerServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
