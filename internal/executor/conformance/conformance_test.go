package conformance_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	// The worker requires a 0700-private socket parent and refuses to listen
	// otherwise, so the socket cannot sit in os.TempDir() (world-writable on
	// Linux CI) or in the umask-masked directory t.TempDir() returns, whose
	// embedded test name also exceeds the macOS sun_path limit. MkdirTemp
	// yields a short directory that is always created 0700.
	socketDir, err := os.MkdirTemp("", "as-conf-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "worker.sock")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$", "-test.v") //nolint:gosec // The fixture intentionally launches this signed test binary.
	cmd.Env = append(os.Environ(), workerSocketEnv+"="+socketPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start conformance worker: %v", err)
	}
	// Reap the child in a goroutine so the socket-wait loop can distinguish an
	// early exit (surface the worker's stderr) from a slow start. Reading cmdErr
	// or stderr only happens-after this close, so both stay race-free.
	var cmdErr error
	exited := make(chan struct{})
	go func() { cmdErr = cmd.Wait(); close(exited) }()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-exited
	})
	// The child is a race-instrumented copy of this test binary; under a loaded
	// CI runner its startup plus socket bind can take several seconds, so keep a
	// generous deadline rather than a tight one that flakes.
	const socketDeadline = 30 * time.Second
	deadline := time.Now().Add(socketDeadline)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		select {
		case <-exited:
			t.Fatalf("conformance worker exited before creating its socket: %v\nstderr:\n%s", cmdErr, stderr.String())
		default:
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			<-exited
			t.Fatalf("conformance worker did not create its socket within %s\nstderr:\n%s", socketDeadline, stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := worker.DialEpisodeWorkerSocket(context.Background(), socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	executor := episodes.NewWorkerExecutor(runtimev1.NewEpisodeWorkerClient(conn), "worker-1", "runtime", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
			"decision_type": "need_more_evidence", "intents": []any{},
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
