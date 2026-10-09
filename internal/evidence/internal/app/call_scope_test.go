package app

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestCallRefusesEveryRequestOutsideTheCapabilityBeforeReserving(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*domain.Envelope)
		options callOptions
		want    domain.ErrorKind
	}{
		{name: "incomplete identity", mutate: func(e *domain.Envelope) { e.AttemptID = "" }, want: domain.InvalidArgument},
		{name: "protocol version", mutate: func(e *domain.Envelope) { e.ProtocolVersion = "2.0" }, want: domain.InvalidArgument},
		{name: "oversized arguments", mutate: func(e *domain.Envelope) { e.ArgumentsJSON = make([]byte, domain.MaxArgumentsBytes+1) }, want: domain.ResourceExhausted},
		{name: "invalid trace", mutate: func(e *domain.Envelope) { e.Traceparent = "garbage" }, want: domain.InvalidArgument},
		{name: "tampered capability", mutate: func(e *domain.Envelope) { e.CapabilityToken = tamperedToken(e.CapabilityToken) }, want: domain.PermissionDenied},
		{name: "missing capability", mutate: func(e *domain.Envelope) { e.CapabilityToken = nil }, want: domain.PermissionDenied},
		{name: "expired capability", mutate: func(*domain.Envelope) {}, options: callOptions{serviceNow: testNow.Add(11 * time.Minute)}, want: domain.PermissionDenied},
		{name: "other episode", mutate: func(e *domain.Envelope) { e.EpisodeID = "episode-2" }, want: domain.PermissionDenied},
		{name: "other tenant", mutate: func(e *domain.Envelope) { e.TenantID = "tenant-2" }, want: domain.PermissionDenied},
		{name: "other attempt", mutate: func(e *domain.Envelope) { e.AttemptID = "attempt-2" }, want: domain.PermissionDenied},
		{name: "other fence", mutate: func(e *domain.Envelope) { e.Fence = 2 }, want: domain.PermissionDenied},
		{name: "fence out of range", mutate: func(e *domain.Envelope) { e.Fence = 1 << 63 }, want: domain.InvalidArgument},
		{name: "other situation", mutate: func(e *domain.Envelope) { e.SituationID = "situation-2" }, want: domain.PermissionDenied},
		{name: "other situation version", mutate: func(e *domain.Envelope) { e.SituationVersion = 2 }, want: domain.PermissionDenied},
		{name: "other entity", mutate: func(e *domain.Envelope) { e.EntityID = "motor-2" }, want: domain.PermissionDenied},
		{name: "arguments naming another entity", mutate: func(e *domain.Envelope) { e.ArgumentsJSON = []byte(`{"entity_id":"motor-2"}`) }, want: domain.PermissionDenied},
		{name: "ungranted tool", mutate: func(e *domain.Envelope) { e.ToolName = "shell.exec" }, want: domain.PermissionDenied},
		{name: "other trace", mutate: func(e *domain.Envelope) {
			e.Traceparent = "00-11111111111111111111111111111111-2222222222222222-01"
		}, want: domain.PermissionDenied},
		{name: "foreign runtime epoch", mutate: func(*domain.Envelope) {}, options: callOptions{runtimeEpoch: "epoch-2"}, want: domain.PermissionDenied},
		{name: "range before the capability", mutate: func(e *domain.Envelope) { e.From.Value = testNow.Add(-2 * time.Hour) }, want: domain.PermissionDenied},
		{name: "range after the capability", mutate: func(e *domain.Envelope) { e.Until.Value = testNow.Add(2 * time.Hour) }, want: domain.PermissionDenied},
		{name: "inverted range", mutate: func(e *domain.Envelope) { e.From.Value, e.Until.Value = e.Until.Value, e.From.Value }, want: domain.PermissionDenied},
		{name: "missing range", mutate: func(e *domain.Envelope) { e.Until = domain.Timestamp{} }, want: domain.PermissionDenied},
		{name: "malformed arguments", mutate: func(e *domain.Envelope) { e.ArgumentsJSON = []byte(`{"entity_id":1}`) }, want: domain.InvalidArgument},
		{name: "arguments beyond the closed schema", mutate: func(e *domain.Envelope) { e.ArgumentsJSON = []byte(`{"entity_id":"motor-1","limit":9999}`) }, want: domain.InvalidArgument},
		{name: "unreadable deadline", mutate: func(e *domain.Envelope) { e.Deadline = domain.Timestamp{Present: true} }, want: domain.InvalidArgument},
		{name: "expired deadline", mutate: func(e *domain.Envelope) { e.Deadline.Value = testNow.Add(-time.Second) }, want: domain.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newCallFixture(t, test.options)
			request := workerEnvelope(fixture.token, "call-1")
			test.mutate(&request)
			_, err := fixture.service.Call(t.Context(), request)
			requireRefusal(t, err, test.want)
			if *fixture.queries != 0 || ledgerRows(t, fixture.db) != 0 {
				t.Fatalf("an out-of-scope call reached the provider (%d queries) or the ledger (%d rows)", *fixture.queries, ledgerRows(t, fixture.db))
			}
		})
	}
}

func TestCallAdmitsTheExactGrantAndPassesOnlyAuthenticatedDimensionsToTheProvider(t *testing.T) {
	t.Parallel()
	var seen Call
	fixture := newCallFixture(t, callOptions{query: func(_ context.Context, call Call) (QueryResult, error) {
		seen = call
		return QueryResult{JSON: []byte(`{"rows":[]}`)}, nil
	}})
	request := workerEnvelope(fixture.token, "call-1")
	request.MaxRows, request.MaxBytes = 500, 50000

	if _, err := fixture.service.Call(t.Context(), request); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if seen.EntityID != "motor-1" || seen.TenantID != "tenant-1" || seen.Trace.Traceparent != testTraceparent {
		t.Fatalf("provider scope = %+v", seen)
	}
	if seen.MaxRows != 10 || seen.MaxBytes != 1024 {
		t.Fatalf("provider budgets = %d rows %d bytes, want the capability's 10/1024: a request cannot widen its grant", seen.MaxRows, seen.MaxBytes)
	}
	if !seen.From.Equal(testNow.Add(-time.Hour)) || !seen.Until.Equal(testNow.Add(time.Hour)) {
		t.Fatalf("provider range = %v..%v", seen.From, seen.Until)
	}
}
