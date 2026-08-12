package conformance_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/conformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFakeExecutorConforms(t *testing.T) {
	if err := conformance.Run(context.Background(), episodes.NewFakeExecutor()); err != nil {
		t.Fatal(err)
	}
}

func TestStreamedWorkerConforms(t *testing.T) {
	decision := map[string]any{
		"decision_id": "dec-conformance", "episode_id": "epi-conformance", "attempt_id": "att-conformance", "fence": 1,
		"snapshot_digest": conformance.FixtureRequest().SnapshotSHA256, "situation_id": "sit-conformance", "situation_version": 1,
		"intents": []any{},
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	srv := &worker.Server{WorkerName: "worker-1", WorkerVersion: "test", ExecuteFunc: func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		if err := emit(&runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(1, 0)), Payload: &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{DecisionJson: decisionJSON, DecisionSha256: digestBytes, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence()}}}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(1, 0)), Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "conformance"}}})
	}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, srv)
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()
	conn, err := grpc.NewClient("passthrough:///conformance", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	executor := episodes.NewWorkerExecutor(runtimev1.NewEpisodeWorkerClient(conn), "worker-1", "runtime", nil)
	if err := conformance.Run(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
}
