package remote_test

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const workerSocketEnv = "AGENTIC_STREAM_REMOTE_WORKER_SOCKET"

func TestMain(m *testing.M) {
	if socketPath := os.Getenv(workerSocketEnv); socketPath != "" {
		os.Exit(serveWorkerOnSocket(socketPath))
	}
	os.Exit(m.Run())
}

func serveWorkerOnSocket(socketPath string) int {
	listener, err := worker.ListenEvidenceSocket(socketPath)
	if err != nil {
		_, _ = os.Stderr.WriteString("listen on worker socket: " + err.Error() + "\n")
		return 1
	}
	defer func() { _ = listener.Close() }()
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, conformingWorker())
	_, _ = os.Stdout.WriteString("listening\n")
	if err := server.Serve(listener); err != nil {
		_, _ = os.Stderr.WriteString("serve worker: " + err.Error() + "\n")
		return 1
	}
	return 0
}

func TestWorkerProcessOnAUnixSocketConformsToTheExecutorPort(t *testing.T) {
	t.Parallel()
	socketPath := startWorkerProcess(t)
	conn, err := worker.DialEpisodeWorkerSocketTLS(t.Context(), socketPath, nil)
	if err != nil {
		t.Fatalf("dial worker socket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	executor := remote.NewExecutor(runtimev1.NewEpisodeWorkerClient(conn), "worker-1", "runtime", nil)
	if err := executorconformance.Run(t.Context(), executor); err != nil {
		t.Fatal(err)
	}
}

func startWorkerProcess(t *testing.T) string {
	t.Helper()
	socketPath := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$") //nolint:gosec // The worker is this test binary, re-executed.
	cmd.Env = append(os.Environ(), workerSocketEnv+"="+socketPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	listening, readerDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(readerDone)
		_, readErr := bufio.NewReader(stdout).ReadString('\n')
		listening <- readErr
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-readerDone
		_ = cmd.Wait()
	})
	awaitListening(t, listening, &stderr)
	return socketPath
}

func awaitListening(t *testing.T, listening <-chan error, stderr *bytes.Buffer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	select {
	case err := <-listening:
		if err != nil {
			t.Fatalf("worker process exited before listening: %v\nstderr:\n%s", err, stderr.String())
		}
	case <-ctx.Done():
		t.Fatalf("worker process did not start listening: %v", ctx.Err())
	}
}
