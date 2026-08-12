package evidence

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCapabilityAndEvidenceScope(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	issuer := &Issuer{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: map[string][]byte{"k1": []byte("01234567890123456789012345678901")}, Now: func() time.Time { return now }}
	scope := Scope{KeyID: "k1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 2, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Tools: []string{"evidence.get"}, NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute), From: now.Add(-time.Hour), Until: now.Add(time.Hour), MaxRows: 10, MaxBytes: 1024, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
	token, err := issuer.Issue(scope)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	verifier := &Verifier{Issuer: "runtime", Audience: "evidence-tools", Keys: issuer.Keys, Now: func() time.Time { return now }}
	server := &Server{Verifier: verifier, Now: func() time.Time { return now }, Query: func(_ context.Context, call Call) (QueryResult, error) {
		if call.Trace.Traceparent == "" || call.EntityID != "motor-1" {
			t.Fatalf("query scope = %+v", call)
		}
		return QueryResult{JSON: []byte(`{"rows":[{"value":42}]}`), RowCount: 1}, nil
	}}
	request := validCall(token)
	result, err := server.Call(t.Context(), request)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	hash := sha256.Sum256(result.GetResultJson())
	if string(result.GetResultJson()) != `{"rows":[{"value":42}]}` || string(result.GetResultSha256()) != string(hash[:]) {
		t.Fatalf("unexpected result: %v", result)
	}
	if status.Code(mustCallError(t, server, validCall(token))) != codes.AlreadyExists {
		t.Fatal("duplicate call id was accepted")
	}

	request = validCall(token)
	request.CallId = "call-2"
	request.TenantId = "other"
	if status.Code(mustCallError(t, server, request)) != codes.PermissionDenied {
		t.Fatal("wrong tenant was accepted")
	}
	request = validCall(token)
	request.CallId = "call-3"
	request.CapabilityToken[0] ^= 1
	if status.Code(mustCallError(t, server, request)) != codes.PermissionDenied {
		t.Fatal("tampered token was accepted")
	}
	request = validCall(token)
	request.CallId = "call-4"
	request.ArgumentsJson = []byte(`{"entity_id":"other"}`)
	if status.Code(mustCallError(t, server, request)) != codes.PermissionDenied {
		t.Fatal("wrong entity was accepted")
	}
}

func TestExpiredCapabilityFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	keys := map[string][]byte{"k1": []byte("01234567890123456789012345678901")}
	issuer := &Issuer{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: func() time.Time { return now }}
	token, err := issuer.Issue(Scope{KeyID: "k1", EpisodeID: "e", AttemptID: "a", Fence: 1, TenantID: "t", SituationID: "s", SituationVersion: 1, EntityID: "x", Tools: []string{"evidence.get"}, NotBefore: now.Add(-time.Hour), ExpiresAt: now.Add(time.Minute), From: now, Until: now.Add(time.Hour), MaxRows: 1, MaxBytes: 10, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &Verifier{Issuer: "runtime", Audience: "evidence-tools", Keys: keys, Now: func() time.Time { return now.Add(2 * time.Minute) }}
	if _, err := verifier.Verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func validCall(token []byte) *runtimev1.EvidenceToolCall {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return &runtimev1.EvidenceToolCall{ProtocolVersion: "1.0", EpisodeId: "episode-1", CallId: "call-1", ToolName: "evidence.get", ArgumentsJson: []byte(`{"entity_id":"motor-1"}`), CapabilityToken: token, Deadline: timestamppb.New(now.Add(time.Minute)), AttemptId: "attempt-1", Fence: 2, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", TenantId: "tenant-1", SituationId: "situation-1", SituationVersion: 1, EntityId: "motor-1", MaxRows: 1, MaxBytes: 100, TimeFrom: timestamppb.New(now.Add(-time.Hour)), TimeUntil: timestamppb.New(now.Add(time.Hour))}
}

func mustCallError(t *testing.T, server *Server, request *runtimev1.EvidenceToolCall) error {
	t.Helper()
	_, err := server.Call(t.Context(), request)
	if err == nil {
		t.Fatal("call unexpectedly succeeded")
	}
	return err
}
