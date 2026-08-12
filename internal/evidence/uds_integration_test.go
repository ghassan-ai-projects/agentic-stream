package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc"
)

func TestEvidenceToolsOverPrivateUDS(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "as-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "evidence.sock")
	listener, err := worker.ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	keys := map[string][]byte{"k1": []byte("01234567890123456789012345678901")}
	issuer := &Issuer{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: func() time.Time { return now }}
	token, err := issuer.Issue(Scope{KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 2, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"}, NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute), From: now.Add(-time.Hour), Until: now.Add(time.Hour), MaxRows: 10, MaxBytes: 1024, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEvidenceToolsServer(server, &Server{Verifier: &Verifier{Issuer: "runtime", Audience: "evidence-tools", Keys: keys, Now: func() time.Time { return now }}, Now: func() time.Time { return now }, Query: func(context.Context, Call) (QueryResult, error) { return QueryResult{JSON: []byte(`{"ok":true}`)}, nil }})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := worker.DialEvidenceSocket(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	request := validCall(token)
	request.CallId = "uds-call"
	result, err := runtimev1.NewEvidenceToolsClient(conn).Call(t.Context(), request)
	if err != nil {
		t.Fatalf("UDS evidence call: %v", err)
	}
	if string(result.GetResultJson()) != `{"ok":true}` {
		t.Fatalf("result = %s", result.GetResultJson())
	}
}
