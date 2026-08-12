package episodes

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestWorkerExecutorConsumesFencedStream(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		decisionJSON := []byte(`{"decision_id":"d-1"}`)
		decisionDigest, _ := canonicaljson.Digest(canonicaljson.DomainDecision, map[string]any{"decision_id": "d-1"})
		decisionHash, _ := canonicaljson.DecodeDigest(decisionDigest)
		if err := emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
				DecisionJson: decisionJSON, DecisionSha256: decisionHash, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			}},
		}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "complete"}},
		})
	})
	executor := NewWorkerExecutor(client, "worker-1", "runtime-1", nil)
	outcome, err := executor.Execute(t.Context(), validWorkerRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outcome.Status != string(AttemptProduced) || outcome.AttemptID != "attempt-1" || outcome.Fence != 7 || string(outcome.DecisionJSON) != `{"decision_id":"d-1"}` {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestWorkerExecutorRequiresTerminal(t *testing.T) {
	client := testWorkerClient(t, func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
		return nil
	})
	_, err := NewWorkerExecutor(client, "worker-1", "runtime-1", nil).Execute(t.Context(), validWorkerRequest())
	if err == nil {
		t.Fatal("expected missing-terminal error")
	}
}

func validWorkerRequest() *Request {
	return &Request{
		EpisodeID: "episode-1", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1,
		ExecutorName: "worker", ExecutorVersion: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		PromptVersion: "prompt-v1", SnapshotSHA256: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		AttemptID: "attempt-1", Fence: 7, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		RequestJSON: []byte(`{"kind":"diagnose","snapshot":{"situation_id":"situation-1"},"tools":[],"risk_ceiling":"R1","trigger":{"trigger_id":"trigger-1","lane":"fast"},"executor":{"objective":"diagnose","decision_schema":{"type":"object"}}}`),
	}
}

func testWorkerClient(t *testing.T, execute worker.ExecuteFunc) runtimev1.EpisodeWorkerClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, &worker.Server{WorkerName: "worker-1", WorkerVersion: "v1", ExecuteFunc: execute})
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() { grpcServer.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial worker: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return runtimev1.NewEpisodeWorkerClient(conn)
}
