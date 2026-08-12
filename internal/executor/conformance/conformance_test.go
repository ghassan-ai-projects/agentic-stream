package conformance_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

const workerSocketEnv = "AGENTIC_STREAM_CONFORMANCE_WORKER_SOCKET"

func TestMain(m *testing.M) {
	if socketPath := os.Getenv(workerSocketEnv); socketPath != "" {
		listener, err := worker.ListenEvidenceSocket(socketPath)
		if err != nil {
			_, _ = os.Stderr.WriteString("listen conformance worker socket: " + err.Error() + "\n")
			os.Exit(1)
		}
		defer func() { _ = listener.Close() }()
		grpcServer := grpc.NewServer()
		runtimev1.RegisterEpisodeWorkerServer(grpcServer, conformanceWorker())
		if err := grpcServer.Serve(listener); err != nil {
			_, _ = os.Stderr.WriteString("serve conformance worker: " + err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestFakeExecutorConforms(t *testing.T) {
	if err := conformance.Run(context.Background(), episodes.NewFakeExecutor()); err != nil {
		t.Fatal(err)
	}
}

func TestStreamedWorkerConforms(t *testing.T) {
	srv := conformanceWorker()
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

func TestSeparateProcessWorkerConforms(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), "as-conformance-"+strconv.Itoa(os.Getpid())+".sock")
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$", "-test.v") //nolint:gosec // The fixture intentionally launches this signed test binary.
	cmd.Env = append(os.Environ(), workerSocketEnv+"="+socketPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start conformance worker: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("conformance worker did not create its socket")
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := worker.DialEpisodeWorkerSocket(context.Background(), socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	executor := episodes.NewWorkerExecutor(runtimev1.NewEpisodeWorkerClient(conn), "worker-1", "runtime", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conformance.Run(ctx, executor); err != nil {
		t.Fatal(err)
	}
}

func conformanceWorker() *worker.Server {
	return &worker.Server{WorkerName: "worker-1", WorkerVersion: "test", ExecuteFunc: func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		decision := map[string]any{
			"decision_id": "dec-conformance", "episode_id": "epi-conformance", "attempt_id": "att-conformance", "fence": 1,
			"snapshot_digest": conformance.FixtureRequest().SnapshotSHA256, "situation_id": "sit-conformance", "situation_version": 1,
			"intents": []any{},
		}
		decisionJSON, err := canonicaljson.Marshal(decision)
		if err != nil {
			return fmt.Errorf("marshal conformance decision: %w", err)
		}
		digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
		if err != nil {
			return fmt.Errorf("digest conformance decision: %w", err)
		}
		digestBytes, err := canonicaljson.DecodeDigest(digest)
		if err != nil {
			return fmt.Errorf("decode conformance decision digest: %w", err)
		}
		if err := emit(&runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(1, 0)), Payload: &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{DecisionJson: decisionJSON, DecisionSha256: digestBytes, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence()}}}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(1, 0)), Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "conformance"}}})
	}}
}
