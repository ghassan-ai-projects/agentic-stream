package transport

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestCallReturnsTheBoundedResultAndReplaysItWithoutQuerying(t *testing.T) {
	t.Parallel()
	fixture := newWorkerFixture(t, okQuery)
	for range 2 {
		result, err := fixture.server.Call(t.Context(), validCall(fixture.token, "call-1"))
		if err != nil || string(result.GetResultJson()) != `{"ok":true}` || len(result.GetResultSha256()) != 32 ||
			result.GetCallId() != "call-1" || result.GetEpisodeId() != "episode-1" || result.GetResultBytes() != 11 {
			t.Fatalf("result = %v, err %v", result, err)
		}
	}
	if *fixture.queries != 1 {
		t.Fatalf("provider queries = %d, want 1: a completed call replays its stored result", *fixture.queries)
	}
}

func TestCallRefusesOutOfScopeRequestsWithoutQueryingTheProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runtimev1.EvidenceToolCall)
		want   codes.Code
	}{
		{"missing request", nil, codes.FailedPrecondition},
		{"another tenant", func(r *runtimev1.EvidenceToolCall) { r.TenantId = "tenant-2" }, codes.PermissionDenied},
		{"another entity", func(r *runtimev1.EvidenceToolCall) {
			r.EntityId = "motor-2"
			r.ArgumentsJson = []byte(`{"entity_id":"motor-2"}`)
		}, codes.PermissionDenied},
		{"arguments naming another entity", func(r *runtimev1.EvidenceToolCall) { r.ArgumentsJson = []byte(`{"entity_id":"motor-2"}`) }, codes.PermissionDenied},
		{"ungranted tool", func(r *runtimev1.EvidenceToolCall) { r.ToolName = "shell.exec" }, codes.PermissionDenied},
		{"tampered capability", func(r *runtimev1.EvidenceToolCall) { r.CapabilityToken[0] ^= 1 }, codes.PermissionDenied},
		{"incomplete identity", func(r *runtimev1.EvidenceToolCall) { r.AttemptId = "" }, codes.InvalidArgument},
		{"closed argument schema", func(r *runtimev1.EvidenceToolCall) { r.ArgumentsJson = []byte(`{"entity_id":"motor-1","sql":"x"}`) }, codes.InvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newWorkerFixture(t, okQuery)
			var request *runtimev1.EvidenceToolCall
			if test.mutate != nil {
				request = validCall(fixture.token, "call-1")
				test.mutate(request)
			}
			_, err := fixture.server.Call(t.Context(), request)
			if got := status.Code(err); got != test.want {
				t.Fatalf("code = %v, want %v (err %v)", got, test.want, err)
			}
			if *fixture.queries != 0 {
				t.Fatal("an out-of-scope call reached the provider")
			}
		})
	}
}

func TestRefusalKindsMapToGRPCCodes(t *testing.T) {
	t.Parallel()
	tests := map[domain.ErrorKind]codes.Code{
		domain.InvalidArgument:    codes.InvalidArgument,
		domain.ResourceExhausted:  codes.ResourceExhausted,
		domain.PermissionDenied:   codes.PermissionDenied,
		domain.FailedPrecondition: codes.FailedPrecondition,
		domain.AlreadyExists:      codes.AlreadyExists,
		domain.Internal:           codes.Internal,
		domain.Canceled:           codes.Canceled,
		domain.DeadlineExceeded:   codes.DeadlineExceeded,
		domain.ErrorKind("novel"): codes.Internal,
	}
	for kind, want := range tests {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			got := status.Convert(statusError(domain.Refuse(kind, "safe message")))
			if got.Code() != want || got.Message() != "safe message" {
				t.Fatalf("status = %v %q, want %v %q", got.Code(), got.Message(), want, "safe message")
			}
		})
	}
}

func TestUnclassifiedErrorsNeverLeakDetailsToTheWorker(t *testing.T) {
	t.Parallel()
	got := status.Convert(statusError(errors.New("private database details")))
	if got.Code() != codes.Internal || got.Message() != "evidence query failed" {
		t.Fatalf("status = %v %q, want Internal with the fixed message", got.Code(), got.Message())
	}
}

func TestProviderFailureReachesTheWorkerAsAFixedInternalError(t *testing.T) {
	t.Parallel()
	fixture := newWorkerFixture(t, func(context.Context, domain.Call) (domain.QueryResult, error) {
		return domain.QueryResult{}, errors.New("sqlite: table events is locked")
	})
	_, err := fixture.server.Call(t.Context(), validCall(fixture.token, "call-1"))
	if got := status.Convert(err); got.Code() != codes.Internal || got.Message() != "evidence query failed" {
		t.Fatalf("status = %v %q, want the fixed Internal message", got.Code(), got.Message())
	}
}

func TestWorkerSeesResultsAndRefusalsOverGRPC(t *testing.T) {
	t.Parallel()
	fixture := newWorkerFixture(t, okQuery)
	listener := bufconn.Listen(1 << 16)
	server := grpc.NewServer()
	runtimev1.RegisterEvidenceToolsServer(server, fixture.server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///evidence", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial evidence tools: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := runtimev1.NewEvidenceToolsClient(conn)

	granted, err := client.Call(t.Context(), validCall(fixture.token, "call-1"))
	if err != nil || string(granted.GetResultJson()) != `{"ok":true}` {
		t.Fatalf("granted call = %v, err %v", granted, err)
	}
	foreign := validCall(fixture.token, "call-2")
	foreign.TenantId = "tenant-2"
	if _, err := client.Call(t.Context(), foreign); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("out-of-scope call error = %v, want PermissionDenied", err)
	}
}
