package domain

import (
	"testing"
	"time"
)

func completeScope() Scope {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return Scope{KeyID: "k1", Issuer: "runtime", Audience: "evidence-tools", EpisodeID: "e", AttemptID: "a", Fence: 1, TenantID: "t", SituationID: "s", SituationVersion: 1, EntityID: "x", Tools: []string{"evidence.get"}, TokenID: "token", IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), From: now.Add(-time.Hour), Until: now, MaxRows: 1, MaxBytes: 100, Traceparent: "trace", RuntimeEpoch: "epoch"}
}

func TestScopeDefaultsAndValidity(t *testing.T) {
	s := completeScope()
	now := s.IssuedAt
	for name, mutate := range map[string]func(*Scope){"identity": func(s *Scope) { s.Fence = 0 }, "window": func(s *Scope) { s.NotBefore = s.ExpiresAt }, "range": func(s *Scope) { s.Until = s.From.Add(-time.Second) }} {
		t.Run(name, func(t *testing.T) {
			candidate := s
			mutate(&candidate)
			if err := ValidateScope(candidate); err == nil {
				t.Fatal("invalid scope accepted")
			}
		})
	}
	if err := ValidateScope(s); err != nil {
		t.Fatal(err)
	}
	s.IssuedAt = time.Time{}
	s.NotBefore = time.Time{}
	s.ExpiresAt = time.Time{}
	got, err := PrepareScope(s, now, DefaultCapabilityTTL)
	if err != nil || !got.ExpiresAt.Equal(now.Add(DefaultCapabilityTTL)) {
		t.Fatalf("defaults=%+v, err=%v", got, err)
	}
	s.Traceparent = ""
	if _, err := PrepareScope(s, now, DefaultCapabilityTTL); err == nil {
		t.Fatal("trace omitted")
	}
}

func TestCapabilityTimeChecks(t *testing.T) {
	s := completeScope()
	now := s.IssuedAt
	for name, mutate := range map[string]func(*Scope){"authority": func(s *Scope) { s.Issuer = "" }, "ttl": func(s *Scope) { s.ExpiresAt = now.Add(time.Hour) }, "expired": func(s *Scope) { s.ExpiresAt = now }} {
		t.Run(name, func(t *testing.T) {
			candidate := s
			mutate(&candidate)
			if err := CheckIssuable(candidate, now, DefaultCapabilityTTL); err == nil {
				t.Fatal("invalid capability issued")
			}
		})
	}
	if err := CheckIssuable(s, now, DefaultCapabilityTTL); err != nil {
		t.Fatal(err)
	}
	for name, at := range map[string]time.Time{"early": now.Add(-2 * time.Second), "expiry": s.ExpiresAt} {
		t.Run(name, func(t *testing.T) {
			if err := CheckValidity(s, at, DefaultCapabilityTTL, time.Second); err == nil {
				t.Fatal("invalid time accepted")
			}
		})
	}
	if err := CheckValidity(s, now.Add(-time.Second), DefaultCapabilityTTL, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := CheckValidity(s, now, time.Second, time.Second); err == nil {
		t.Fatal("excess TTL accepted")
	}
}
