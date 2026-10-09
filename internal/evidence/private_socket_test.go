package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestEvidenceToolsServeWorkersOverThePrivateSocket(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "as-") //nolint:usetesting // t.TempDir paths can exceed the 104 byte unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "evidence.sock")
	listener, err := worker.ListenEvidenceSocket(path)
	if err != nil {
		t.Fatalf("listen on the private socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	capabilities := newCapabilities(t)
	token, err := capabilities.Issue(facadeScope())
	if err != nil {
		t.Fatalf("issue capability: %v", err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEvidenceToolsServer(server, newCallService(t, capabilities, func(context.Context, Call) (QueryResult, error) {
		return QueryResult{JSON: []byte(`{"ok":true}`)}, nil
	}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := workerfake.DialEvidenceSocket(t.Context(), path)
	if err != nil {
		t.Fatalf("dial the private socket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := runtimev1.NewEvidenceToolsClient(conn)

	granted := facadeCall(token)
	granted.CallId = "uds-call"
	result, err := client.Call(t.Context(), granted)
	if err != nil || string(result.GetResultJson()) != `{"ok":true}` {
		t.Fatalf("granted call = %v, err %v", result, err)
	}
	escaped := facadeCall(token)
	escaped.CallId, escaped.EntityId = "uds-escape", "motor-2"
	if _, err := client.Call(t.Context(), escaped); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("out-of-scope call error = %v, want PermissionDenied", err)
	}
}
