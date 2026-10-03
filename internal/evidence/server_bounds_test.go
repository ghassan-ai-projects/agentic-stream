package evidence

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var boundsNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

const boundsTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

// boundsScope matches the episode and attempt that openLedgerDB seeds.
func boundsScope() Scope {
	return Scope{KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"}, NotBefore: boundsNow, ExpiresAt: boundsNow.Add(10 * time.Minute), From: boundsNow.Add(-time.Hour), Until: boundsNow.Add(time.Hour), MaxRows: 10, MaxBytes: 1024, Traceparent: boundsTraceparent, RuntimeEpoch: "epoch-1"}
}

func boundsServer(t *testing.T, query Query) (*Server, []byte) {
	t.Helper()
	keys := map[string][]byte{"k1": []byte("01234567890123456789012345678901")}
	clock := func() time.Time { return boundsNow }
	issuer := &Issuer{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: clock}
	token, err := issuer.Issue(boundsScope())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	verifier := &Verifier{Issuer: "runtime", Audience: "evidence-tools", Keys: keys, Now: clock}
	return &Server{Verifier: verifier, Now: clock, Query: query}, token
}

func boundsCall(token []byte, callID string) *runtimev1.EvidenceToolCall {
	return &runtimev1.EvidenceToolCall{ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: callID, ToolName: "evidence.get", ArgumentsJson: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token, Deadline: timestamppb.New(boundsNow.Add(time.Minute)), AttemptId: "attempt-1", Fence: 1, Traceparent: boundsTraceparent, TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100, TimeFrom: timestamppb.New(boundsNow.Add(-time.Hour)), TimeUntil: timestamppb.New(boundsNow.Add(time.Hour))}
}

func okQuery(_ context.Context, _ Call) (QueryResult, error) {
	return QueryResult{JSON: []byte(`{"rows":[]}`), RowCount: 0}, nil
}

func TestCallRefusesEveryOutOfScopeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*runtimev1.EvidenceToolCall)
		epoch  string
		want   codes.Code
	}{
		{name: "incomplete identity", mutate: func(r *runtimev1.EvidenceToolCall) { r.AttemptId = "" }, want: codes.InvalidArgument},
		{name: "protocol version", mutate: func(r *runtimev1.EvidenceToolCall) { r.ProtocolVersion = "2.0" }, want: codes.InvalidArgument},
		{name: "oversized arguments", mutate: func(r *runtimev1.EvidenceToolCall) { r.ArgumentsJson = make([]byte, maxArgumentsBytes+1) }, want: codes.ResourceExhausted},
		{name: "invalid trace", mutate: func(r *runtimev1.EvidenceToolCall) { r.Traceparent = "garbage" }, want: codes.InvalidArgument},
		{name: "other attempt", mutate: func(r *runtimev1.EvidenceToolCall) { r.AttemptId = "attempt-2" }, want: codes.PermissionDenied},
		{name: "other fence", mutate: func(r *runtimev1.EvidenceToolCall) { r.Fence = 2 }, want: codes.PermissionDenied},
		{name: "fence out of range", mutate: func(r *runtimev1.EvidenceToolCall) { r.Fence = 1 << 63 }, want: codes.InvalidArgument},
		{name: "other situation version", mutate: func(r *runtimev1.EvidenceToolCall) { r.SituationVersion = 2 }, want: codes.PermissionDenied},
		{name: "other situation", mutate: func(r *runtimev1.EvidenceToolCall) { r.SituationId = "situation-2" }, want: codes.PermissionDenied},
		{name: "ungranted tool", mutate: func(r *runtimev1.EvidenceToolCall) { r.ToolName = "shell.exec" }, want: codes.PermissionDenied},
		{name: "other trace", mutate: func(r *runtimev1.EvidenceToolCall) {
			r.Traceparent = "00-11111111111111111111111111111111-2222222222222222-01"
		}, want: codes.PermissionDenied},
		{name: "foreign runtime epoch", mutate: func(*runtimev1.EvidenceToolCall) {}, epoch: "epoch-2", want: codes.PermissionDenied},
		{name: "range before capability", mutate: func(r *runtimev1.EvidenceToolCall) { r.TimeFrom = timestamppb.New(boundsNow.Add(-2 * time.Hour)) }, want: codes.PermissionDenied},
		{name: "range after capability", mutate: func(r *runtimev1.EvidenceToolCall) { r.TimeUntil = timestamppb.New(boundsNow.Add(2 * time.Hour)) }, want: codes.PermissionDenied},
		{name: "inverted range", mutate: func(r *runtimev1.EvidenceToolCall) {
			r.TimeFrom, r.TimeUntil = r.TimeUntil, r.TimeFrom
		}, want: codes.PermissionDenied},
		{name: "missing range", mutate: func(r *runtimev1.EvidenceToolCall) { r.TimeUntil = nil }, want: codes.PermissionDenied},
		{name: "malformed arguments", mutate: func(r *runtimev1.EvidenceToolCall) { r.ArgumentsJson = []byte(`{"entity_id":1}`) }, want: codes.InvalidArgument},
		{name: "invalid deadline", mutate: func(r *runtimev1.EvidenceToolCall) { r.Deadline = &timestamppb.Timestamp{Nanos: -1} }, want: codes.InvalidArgument},
		{name: "expired deadline", mutate: func(r *runtimev1.EvidenceToolCall) { r.Deadline = timestamppb.New(boundsNow.Add(-time.Second)) }, want: codes.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queried := false
			server, token := boundsServer(t, func(ctx context.Context, call Call) (QueryResult, error) {
				queried = true
				return okQuery(ctx, call)
			})
			server.RuntimeEpoch = tt.epoch
			request := boundsCall(token, "call-1")
			tt.mutate(request)
			_, err := server.Call(t.Context(), request)
			if got := status.Code(err); got != tt.want {
				t.Fatalf("code = %v, want %v (err %v)", got, tt.want, err)
			}
			if queried {
				t.Fatal("an out-of-scope call reached the query")
			}
		})
	}
}

func TestCallEnforcesResultBoundsAndRecordsFailures(t *testing.T) {
	tests := []struct {
		name     string
		result   QueryResult
		queryErr error
		want     codes.Code
		wantCode string
	}{
		{name: "too many bytes", result: QueryResult{JSON: make([]byte, 101)}, want: codes.ResourceExhausted, wantCode: "result_bytes_exceeded"},
		{name: "too many rows", result: QueryResult{JSON: []byte(`[]`), RowCount: 2}, want: codes.ResourceExhausted, wantCode: "result_rows_exceeded"},
		{name: "query failure", queryErr: errors.New("database is gone"), want: codes.Internal, wantCode: "query_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openLedgerDB(t)
			server, token := boundsServer(t, func(context.Context, Call) (QueryResult, error) { return tt.result, tt.queryErr })
			server.RuntimeEpoch = "epoch-1"
			server.Ledger = &Ledger{DB: db, LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Lease: time.Minute, Now: fixedLedgerClock()}

			if _, err := server.Call(t.Context(), boundsCall(token, "call-1")); status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err %v)", status.Code(err), tt.want, err)
			}
			if gotStatus, gotCode := readLedgerStatus(t, db, "call-1"); gotStatus != "failed" || gotCode != tt.wantCode {
				t.Fatalf("ledger status=%q code=%q, want failed/%s", gotStatus, gotCode, tt.wantCode)
			}
			if _, err := server.Call(t.Context(), boundsCall(token, "call-1")); status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("retrying a terminal call = %v, want FailedPrecondition", err)
			}
		})
	}
}

func TestLedgerReplaysCompletedCallWithoutQuerying(t *testing.T) {
	db := openLedgerDB(t)
	queries := 0
	server, token := boundsServer(t, func(context.Context, Call) (QueryResult, error) {
		queries++
		return QueryResult{JSON: []byte(`{"rows":[{"value":7}]}`), RowCount: 1}, nil
	})
	server.RuntimeEpoch = "epoch-1"
	server.Ledger = &Ledger{DB: db, LeaseOwner: "owner-1", RuntimeEpoch: "epoch-1", Lease: time.Minute, Now: fixedLedgerClock()}

	first, err := server.Call(t.Context(), boundsCall(token, "call-1"))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := server.Call(t.Context(), boundsCall(token, "call-1"))
	if err != nil {
		t.Fatalf("replayed call: %v", err)
	}
	if queries != 1 {
		t.Fatalf("queries = %d; a completed call must replay its stored result", queries)
	}
	if string(first.GetResultSha256()) != string(second.GetResultSha256()) || string(second.GetResultJson()) != `{"rows":[{"value":7}]}` {
		t.Fatalf("replayed result differs: %v vs %v", first, second)
	}
}

func TestInMemoryCallIdentityIsReleasedOnlyOnQueryFailure(t *testing.T) {
	t.Parallel()

	fail := true
	server, token := boundsServer(t, func(ctx context.Context, call Call) (QueryResult, error) {
		if fail {
			return QueryResult{}, errors.New("transient")
		}
		return okQuery(ctx, call)
	})
	if _, err := server.Call(t.Context(), boundsCall(token, "call-1")); status.Code(err) != codes.Internal {
		t.Fatalf("failed query = %v", err)
	}
	fail = false
	if _, err := server.Call(t.Context(), boundsCall(token, "call-1")); err != nil {
		t.Fatalf("retry after a failed query: %v", err)
	}
	if _, err := server.Call(t.Context(), boundsCall(token, "call-1")); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("reuse after success = %v, want AlreadyExists", err)
	}
}
