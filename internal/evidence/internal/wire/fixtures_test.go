package wire

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

var signingKey = []byte("01234567890123456789012345678901")

var scopeTime = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func signedScope() domain.Scope {
	return domain.Scope{
		KeyID: "k1", Issuer: "runtime", Audience: "tools", TokenID: "fixed",
		EpisodeID: "e", AttemptID: "a", Fence: 1, TenantID: "t", SituationID: "s", SituationVersion: 1, EntityID: "x",
		Tools: []string{"evidence.get"}, IssuedAt: scopeTime, NotBefore: scopeTime, ExpiresAt: scopeTime.Add(time.Minute),
		From: scopeTime.Add(-time.Hour), Until: scopeTime, MaxRows: 1, MaxBytes: 100, Traceparent: "trace", Tracestate: "vendor=1", RuntimeEpoch: "epoch",
	}
}

func fingerprintCall() domain.Call {
	return domain.Call{
		EpisodeID: "episode-1", CallID: "call-1", ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1",
		SituationVersion: 1, EntityID: "motor-1", Arguments: domain.EvidenceGetArguments{EntityID: "motor-1"},
		Deadline: time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), AttemptID: "attempt-1", Fence: 1,
		Trace:   contractsv1.TraceContext{Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		MaxRows: 1, MaxBytes: 100,
		From:  time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
	}
}

func requireError(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was accepted", what)
	}
}
