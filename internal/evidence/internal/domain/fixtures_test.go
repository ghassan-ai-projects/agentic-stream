package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var issuedAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func completeScope() Scope {
	return Scope{
		KeyID: "k1", Issuer: "runtime", Audience: "evidence-tools",
		EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1,
		TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1",
		Tools: []string{"evidence.get"}, TokenID: "token",
		IssuedAt: issuedAt, NotBefore: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
		From: issuedAt.Add(-time.Hour), Until: issuedAt, MaxRows: 10, MaxBytes: 100,
		Traceparent: "trace", RuntimeEpoch: "epoch",
	}
}

func envelopeFor(scope Scope) Envelope {
	return Envelope{
		ProtocolVersion: "1.0", EpisodeID: scope.EpisodeID, CallID: "call-1", ToolName: scope.Tools[0],
		TenantID: scope.TenantID, SituationID: scope.SituationID, EntityID: scope.EntityID,
		AttemptID: scope.AttemptID, Fence: 1, SituationVersion: 1,
		MaxRows: 1, MaxBytes: 50, ArgumentsJSON: []byte(`{"entity_id":"motor-1"}`),
		Traceparent: scope.Traceparent,
		From:        Timestamp{Value: scope.From, Present: true, Valid: true},
		Until:       Timestamp{Value: scope.Until, Present: true, Valid: true},
	}
}

func requireRefusal(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Kind != want {
		t.Fatalf("error = %v, want a %s refusal", err, want)
	}
}

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}
