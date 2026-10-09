package domain

import (
	"testing"
	"time"
)

func TestValidateScopeRequiresACompleteUnambiguousScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Scope)
		want   string
	}{
		{"key", func(s *Scope) { s.KeyID = "" }, "scope is incomplete"},
		{"episode", func(s *Scope) { s.EpisodeID = "" }, "scope is incomplete"},
		{"attempt", func(s *Scope) { s.AttemptID = "" }, "scope is incomplete"},
		{"tenant", func(s *Scope) { s.TenantID = "" }, "scope is incomplete"},
		{"situation", func(s *Scope) { s.SituationID = "" }, "scope is incomplete"},
		{"entity", func(s *Scope) { s.EntityID = "" }, "scope is incomplete"},
		{"fence", func(s *Scope) { s.Fence = 0 }, "scope is incomplete"},
		{"tools", func(s *Scope) { s.Tools = nil }, "scope is incomplete"},
		{"row budget", func(s *Scope) { s.MaxRows = 0 }, "scope is incomplete"},
		{"byte budget", func(s *Scope) { s.MaxBytes = 0 }, "scope is incomplete"},
		{"expiry", func(s *Scope) { s.ExpiresAt = time.Time{} }, "validity window is invalid"},
		{"not before", func(s *Scope) { s.NotBefore = time.Time{} }, "validity window is invalid"},
		{"issued at", func(s *Scope) { s.IssuedAt = time.Time{} }, "validity window is invalid"},
		{"issued after not-before", func(s *Scope) { s.IssuedAt = s.NotBefore.Add(time.Second) }, "validity window is invalid"},
		{"empty window", func(s *Scope) { s.NotBefore = s.ExpiresAt }, "validity window is invalid"},
		{"range start", func(s *Scope) { s.From = time.Time{} }, "evidence range is invalid"},
		{"range end", func(s *Scope) { s.Until = time.Time{} }, "evidence range is invalid"},
		{"inverted range", func(s *Scope) { s.Until = s.From.Add(-time.Second) }, "evidence range is invalid"},
		{"trace", func(s *Scope) { s.Traceparent = "" }, "evidence range is invalid"},
		{"situation version", func(s *Scope) { s.SituationVersion = 0 }, "evidence range is invalid"},
		{"token identity", func(s *Scope) { s.TokenID = "" }, "evidence range is invalid"},
		{"runtime epoch", func(s *Scope) { s.RuntimeEpoch = "" }, "evidence range is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := completeScope()
			test.mutate(&scope)
			requireErrorContaining(t, ValidateScope(scope), test.want)
		})
	}
	t.Run("complete scope", func(t *testing.T) {
		t.Parallel()
		if err := ValidateScope(completeScope()); err != nil {
			t.Fatalf("ValidateScope: %v", err)
		}
	})
}

func TestPrepareScopeDefaultsTheValidityWindowAndRequiresATrace(t *testing.T) {
	t.Parallel()
	now := issuedAt
	t.Run("zero window defaults from now", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		scope.IssuedAt, scope.NotBefore, scope.ExpiresAt = time.Time{}, time.Time{}, time.Time{}
		got, err := PrepareScope(scope, now, DefaultCapabilityTTL)
		if err != nil || !got.IssuedAt.Equal(now) || !got.NotBefore.Equal(now) || !got.ExpiresAt.Equal(now.Add(DefaultCapabilityTTL)) {
			t.Fatalf("scope = issued %v, not before %v, expires %v, err %v", got.IssuedAt, got.NotBefore, got.ExpiresAt, err)
		}
	})
	t.Run("explicit window is kept", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		got, err := PrepareScope(scope, now.Add(time.Hour), DefaultCapabilityTTL)
		if err != nil || !got.IssuedAt.Equal(scope.IssuedAt) || !got.ExpiresAt.Equal(scope.ExpiresAt) {
			t.Fatalf("scope window changed: %+v, err %v", got, err)
		}
	})
	t.Run("missing trace", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		scope.Traceparent = ""
		_, err := PrepareScope(scope, now, DefaultCapabilityTTL)
		requireErrorContaining(t, err, "traceparent is required")
	})
}

func TestCheckIssuableBoundsTheLifetimeAndNamesTheAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Scope)
		want   string
	}{
		{"missing issuer", func(s *Scope) { s.Issuer = "" }, "issuer and audience are required"},
		{"missing audience", func(s *Scope) { s.Audience = "" }, "issuer and audience are required"},
		{"lifetime above the maximum", func(s *Scope) { s.ExpiresAt = s.IssuedAt.Add(DefaultCapabilityTTL + time.Nanosecond) }, "lifetime exceeds maximum"},
		{"already expired", func(s *Scope) { s.ExpiresAt = issuedAt }, "already expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := completeScope()
			test.mutate(&scope)
			requireErrorContaining(t, CheckIssuable(scope, issuedAt, DefaultCapabilityTTL), test.want)
		})
	}
	t.Run("lifetime exactly at the maximum", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		scope.ExpiresAt = scope.IssuedAt.Add(DefaultCapabilityTTL)
		if err := CheckIssuable(scope, issuedAt, DefaultCapabilityTTL); err != nil {
			t.Fatalf("CheckIssuable: %v", err)
		}
	})
}

func TestCheckValidityEnforcesTheWindowWithBoundedSkew(t *testing.T) {
	t.Parallel()
	scope := completeScope()
	const skew = time.Second
	tests := []struct {
		name    string
		now     time.Time
		maxTTL  time.Duration
		wantErr string
	}{
		{"inside the window", scope.IssuedAt, DefaultCapabilityTTL, ""},
		{"before not-before within skew", scope.NotBefore.Add(-skew), DefaultCapabilityTTL, ""},
		{"before not-before beyond skew", scope.NotBefore.Add(-skew - time.Nanosecond), DefaultCapabilityTTL, "outside its validity window"},
		{"last instant before expiry", scope.ExpiresAt.Add(-time.Nanosecond), DefaultCapabilityTTL, ""},
		{"at expiry", scope.ExpiresAt, DefaultCapabilityTTL, "outside its validity window"},
		{"lifetime above the verifier maximum", scope.IssuedAt, 30 * time.Second, "lifetime exceeds maximum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := CheckValidity(scope, test.now, test.maxTTL, skew)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckValidity: %v", err)
				}
				return
			}
			requireErrorContaining(t, err, test.wantErr)
		})
	}
	t.Run("issued in the future beyond skew", func(t *testing.T) {
		t.Parallel()
		now := scope.NotBefore
		future := scope
		future.IssuedAt = now.Add(skew + time.Nanosecond)
		future.ExpiresAt = future.IssuedAt.Add(time.Minute)
		requireErrorContaining(t, CheckValidity(future, now, DefaultCapabilityTTL, skew), "outside its validity window")
	})
}

func TestCheckAuthorityRequiresTheExactIssuerAndAudience(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		issuer, audience string
		wantErr          bool
	}{
		{"configured authority", "runtime", "evidence-tools", false},
		{"foreign issuer", "other", "evidence-tools", true},
		{"foreign audience", "runtime", "other", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := CheckAuthority(completeScope(), test.issuer, test.audience)
			if (err != nil) != test.wantErr {
				t.Fatalf("CheckAuthority error = %v, want error %v", err, test.wantErr)
			}
		})
	}
}

func TestCompleteScopeBindsTheConfiguredAuthority(t *testing.T) {
	t.Parallel()
	authority := Authority{Issuer: "configured-issuer", Audience: "configured-audience", MaxTTL: DefaultCapabilityTTL}
	t.Run("fills a missing issuer and audience", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		scope.Issuer, scope.Audience = "", ""
		got, err := CompleteScope(scope, authority, issuedAt)
		if err != nil || got.Issuer != "configured-issuer" || got.Audience != "configured-audience" {
			t.Fatalf("scope = %q/%q, err %v", got.Issuer, got.Audience, err)
		}
	})
	t.Run("keeps an explicit issuer and audience", func(t *testing.T) {
		t.Parallel()
		got, err := CompleteScope(completeScope(), authority, issuedAt)
		if err != nil || got.Issuer != "runtime" || got.Audience != "evidence-tools" {
			t.Fatalf("scope = %q/%q, err %v", got.Issuer, got.Audience, err)
		}
	})
	t.Run("refuses an incomplete scope", func(t *testing.T) {
		t.Parallel()
		scope := completeScope()
		scope.Fence = 0
		_, err := CompleteScope(scope, authority, issuedAt)
		requireErrorContaining(t, err, "scope is incomplete")
	})
	t.Run("refuses a lifetime beyond the authority maximum", func(t *testing.T) {
		t.Parallel()
		_, err := CompleteScope(completeScope(), Authority{MaxTTL: time.Second}, issuedAt)
		requireErrorContaining(t, err, "lifetime exceeds maximum")
	})
}
