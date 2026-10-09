package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
)

func TestIssuedCapabilityVerifiesToTheGrantedScope(t *testing.T) {
	t.Parallel()
	service := capabilityService(t, testNow)
	token, err := service.Issue(grantedScope())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := service.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	want := grantedScope()
	want.Issuer, want.Audience, want.IssuedAt, want.TokenID = "runtime", "evidence-tools", testNow, got.TokenID
	if got.TokenID == "" || !reflect.DeepEqual(got, want) {
		t.Fatalf("verified scope = %+v\nwant           %+v", got, want)
	}
}

func TestIssueDefaultsTheKeyLifetimeAndMintsADistinctTokenIdentity(t *testing.T) {
	t.Parallel()
	service := capabilityService(t, testNow)
	scope := grantedScope()
	scope.KeyID, scope.NotBefore, scope.ExpiresAt = "", time.Time{}, time.Time{}
	first, err := service.Issue(scope)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	second, err := service.Issue(scope)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := service.Verify(first)
	if err != nil || got.KeyID != "k1" || !got.NotBefore.Equal(testNow) || !got.ExpiresAt.Equal(testNow.Add(15*time.Minute)) {
		t.Fatalf("verified = key %q window %v..%v, err %v, want the configured key and a 15 minute window", got.KeyID, got.NotBefore, got.ExpiresAt, err)
	}
	other, err := service.Verify(second)
	if err != nil || other.TokenID == got.TokenID {
		t.Fatalf("token identities %q and %q, want distinct (err %v)", got.TokenID, other.TokenID, err)
	}
}

func TestIssueRefusesScopesItCannotBindToAWorker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Scope)
		want   string
	}{
		{"no trace", func(s *Scope) { s.Traceparent = "" }, "traceparent is required"},
		{"no fence", func(s *Scope) { s.Fence = 0 }, "scope is incomplete"},
		{"no tools", func(s *Scope) { s.Tools = nil }, "scope is incomplete"},
		{"no runtime epoch", func(s *Scope) { s.RuntimeEpoch = "" }, "evidence range is invalid"},
		{"unknown signing key", func(s *Scope) { s.KeyID = "k9" }, `signing key "k9" is unavailable`},
		{"lifetime beyond the maximum", func(s *Scope) { s.ExpiresAt = testNow.Add(16 * time.Minute) }, "lifetime exceeds maximum"},
		{"already expired", func(s *Scope) {
			s.IssuedAt, s.NotBefore, s.ExpiresAt = testNow.Add(-2*time.Minute), testNow.Add(-2*time.Minute), testNow
		}, "already expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := grantedScope()
			test.mutate(&scope)
			_, err := capabilityService(t, testNow).Issue(scope)
			requireContains(t, err, test.want)
		})
	}
}

func TestIssueRefusesAKeyBelowTheMinimumLength(t *testing.T) {
	t.Parallel()
	service := capabilityService(t, testNow)
	service.issuer.Keys["k1"] = []byte("short")
	_, err := service.Issue(grantedScope())
	requireContains(t, err, "unavailable or too short")
}

func TestVerifyRefusesTokensOutsideTheConfiguredAuthorityAndWindow(t *testing.T) {
	t.Parallel()
	token, err := capabilityService(t, testNow).Issue(grantedScope())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	tests := []struct {
		name     string
		verifier *Service
		token    []byte
		want     string
	}{
		{"another issuer", capabilityServiceWith(t, CapabilityConfig{Issuer: "other", Now: fixedClock(testNow)}), token, "issuer or audience mismatch"},
		{"another audience", capabilityServiceWith(t, CapabilityConfig{Audience: "other", Now: fixedClock(testNow)}), token, "issuer or audience mismatch"},
		{"another key ring", capabilityServiceWith(t, CapabilityConfig{Keys: map[string][]byte{"k1": []byte("abcdefghijklmnopqrstuvwxyz012345")}, Now: fixedClock(testNow)}), token, "signature"},
		{"unknown key identity", capabilityServiceWith(t, CapabilityConfig{KeyID: "k2", Keys: map[string][]byte{"k2": []byte(testKey)}, Now: fixedClock(testNow)}), token, "unknown capability key"},
		{"before not-before beyond skew", capabilityService(t, testNow.Add(-2*time.Second)), token, "outside its validity window"},
		{"at expiry", capabilityService(t, testNow.Add(10*time.Minute)), token, "outside its validity window"},
		{"lifetime above the verifier maximum", capabilityServiceWith(t, CapabilityConfig{MaxTTL: time.Minute, Now: fixedClock(testNow)}), token, "lifetime exceeds maximum"},
		{"tampered token", capabilityService(t, testNow), tamperedToken(token), "invalid capability token signature"},
		{"empty token", capabilityService(t, testNow), nil, "invalid capability token format"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope, err := test.verifier.Verify(test.token)
			requireContains(t, err, test.want)
			if scope.EpisodeID != "" {
				t.Fatalf("a refused token yielded scope %+v", scope)
			}
		})
	}
}

func TestVerifyHonorsTheConfiguredClockSkew(t *testing.T) {
	t.Parallel()
	token, err := capabilityService(t, testNow).Issue(grantedScope())
	if err != nil {
		t.Fatal(err)
	}
	early := testNow.Add(-5 * time.Second)
	if _, err := capabilityServiceWith(t, CapabilityConfig{ClockSkew: 10 * time.Second, Now: fixedClock(early)}).Verify(token); err != nil {
		t.Fatalf("a wide skew refused a slightly early token: %v", err)
	}
	if _, err := capabilityService(t, early).Verify(token); err == nil {
		t.Fatal("the default one second skew accepted a token five seconds early")
	}
}

func TestVerifyRefusesAnIncompleteSignedScope(t *testing.T) {
	t.Parallel()
	service := capabilityService(t, testNow)
	scope := grantedScope()
	scope.IssuedAt, scope.TokenID, scope.Issuer, scope.Audience = testNow, "token", "runtime", "evidence-tools"
	scope.RuntimeEpoch = ""
	token, err := wire.SignToken(scope, []byte(testKey))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Verify(token)
	requireContains(t, err, "evidence range is invalid")
}
