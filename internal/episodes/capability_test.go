package episodes

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

func TestAttemptCapabilityIssuerBindsRequestIdentity(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	keys := map[string][]byte{"k1": []byte("01234567890123456789012345678901")}
	issuer := &evidence.Issuer{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: func() time.Time { return now }}
	factory := &AttemptCapabilityIssuer{Issuer: issuer, RuntimeEpoch: "epoch-1", Tools: []string{"evidence.get"}, From: now.Add(-time.Hour), Until: now, MaxRows: 10, MaxBytes: 1024, ExpiresAt: now.Add(10 * time.Minute)}
	token, err := factory.Issue(&Request{EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 2, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 3, EntityID: "motor-1", Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	scope, err := (&evidence.Verifier{Issuer: "runtime", Audience: "evidence-tools", Keys: keys, Now: func() time.Time { return now }}).Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if scope.EpisodeID != "episode-1" || scope.AttemptID != "attempt-1" || scope.Fence != 2 || scope.EntityID != "motor-1" || scope.RuntimeEpoch != "epoch-1" {
		t.Fatalf("scope = %+v", scope)
	}
}
