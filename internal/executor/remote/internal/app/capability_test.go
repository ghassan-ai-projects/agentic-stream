package app

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

var capabilityNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func capabilityConfig(maxTTL time.Duration) evidence.CapabilityConfig {
	return evidence.CapabilityConfig{
		Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", MaxTTL: maxTTL,
		Keys: map[string][]byte{"k1": []byte("01234567890123456789012345678901")},
		Now:  func() time.Time { return capabilityNow },
	}
}

func newCapabilityService(t *testing.T, cfg evidence.CapabilityConfig) *evidence.Service {
	t.Helper()
	service, err := evidence.New(evidence.Config{Capabilities: &cfg})
	if err != nil {
		t.Fatalf("evidence service: %v", err)
	}
	return service
}

func newIssuer(service *evidence.Service) *AttemptCapabilityIssuer {
	return &AttemptCapabilityIssuer{
		Issuer: service, RuntimeEpoch: "epoch-1", Tools: []string{"evidence.get"},
		From: capabilityNow.Add(-time.Hour), Until: capabilityNow, MaxRows: 10, MaxBytes: 1024,
		ExpiresAt: capabilityNow.Add(10 * time.Minute),
	}
}

func attemptRequest() *episodes.Request {
	return &episodes.Request{
		EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 2, TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 3,
		EntityID: "motor-1", Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", Tracestate: "vendor=1",
	}
}

func TestAttemptCapabilityIssuerBindsRequestIdentityAndScope(t *testing.T) {
	t.Parallel()
	service := newCapabilityService(t, capabilityConfig(0))
	token, err := newIssuer(service).Issue(attemptRequest())
	if err != nil {
		t.Fatalf("Issue() = %v", err)
	}
	scope, err := service.Verify(token)
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	identity := scope.EpisodeID == "episode-1" && scope.AttemptID == "attempt-1" && scope.Fence == 2 && scope.TenantID == "tenant-1" &&
		scope.SituationID == "situation-1" && scope.SituationVersion == 3 && scope.EntityID == "motor-1" && scope.RuntimeEpoch == "epoch-1"
	if !identity || scope.Traceparent != attemptRequest().Traceparent || scope.Tracestate != "vendor=1" {
		t.Fatalf("scope identity = %+v", scope)
	}
	if len(scope.Tools) != 1 || scope.Tools[0] != "evidence.get" || scope.MaxRows != 10 || scope.MaxBytes != 1024 {
		t.Fatalf("scope bounds = %+v", scope)
	}
}

func TestAttemptCapabilityExpiryFollowsTheIssuerMaximumWhenUnset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		maxTTL time.Duration
		want   time.Duration
	}{
		{"unset maximum", 0, 15 * time.Minute},
		{"shorter maximum", 5 * time.Minute, 5 * time.Minute},
		{"explicit maximum", time.Hour, time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := newCapabilityService(t, capabilityConfig(tt.maxTTL))
			issuer := newIssuer(service)
			issuer.ExpiresAt = time.Time{}
			token, err := issuer.Issue(attemptRequest())
			if err != nil {
				t.Fatalf("Issue() = %v", err)
			}
			scope, err := service.Verify(token)
			if err != nil || !scope.ExpiresAt.Equal(capabilityNow.Add(tt.want)) {
				t.Fatalf("expires %v (err %v), want %v", scope.ExpiresAt, err, capabilityNow.Add(tt.want))
			}
		})
	}
}

func TestAttemptCapabilityIssuerFailsClosedWithoutAFullScope(t *testing.T) {
	t.Parallel()
	service := newCapabilityService(t, capabilityConfig(0))
	tests := []struct {
		name    string
		breakIt func(*AttemptCapabilityIssuer, *episodes.Request)
		want    string
	}{
		{"missing episode", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.EpisodeID = "" }, "scope is incomplete"},
		{"missing attempt", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.AttemptID = "" }, "scope is incomplete"},
		{"missing fence", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.Fence = 0 }, "scope is incomplete"},
		{"missing tenant", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.TenantID = "" }, "scope is incomplete"},
		{"missing situation", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.SituationID = "" }, "scope is incomplete"},
		{"missing situation version", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.SituationVersion = 0 }, "scope is incomplete"},
		{"missing entity", func(_ *AttemptCapabilityIssuer, r *episodes.Request) { r.EntityID = "" }, "scope is incomplete"},
		{"missing runtime epoch", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.RuntimeEpoch = "" }, "scope is incomplete"},
		{"no tools", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.Tools = nil }, "scope is incomplete"},
		{"no evidence window", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.From = time.Time{} }, "scope is incomplete"},
		{"no row bound", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.MaxRows = 0 }, "scope is incomplete"},
		{"no byte bound", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.MaxBytes = 0 }, "scope is incomplete"},
		{"no evidence service", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.Issuer = nil }, "not configured"},
		{"expiry beyond the issuer maximum", func(i *AttemptCapabilityIssuer, _ *episodes.Request) { i.ExpiresAt = capabilityNow.Add(24 * time.Hour) }, "issue attempt capability"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issuer, req := newIssuer(service), attemptRequest()
			tt.breakIt(issuer, req)
			token, err := issuer.Issue(req)
			if err == nil || !strings.Contains(err.Error(), tt.want) || token != nil {
				t.Fatalf("Issue() = %q, %v; want no token and error containing %q", token, err, tt.want)
			}
		})
	}
	var missing *AttemptCapabilityIssuer
	if _, err := missing.Issue(attemptRequest()); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("nil issuer: Issue() = %v", err)
	}
	if _, err := newIssuer(service).Issue(nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("nil request: Issue() = %v", err)
	}
}
