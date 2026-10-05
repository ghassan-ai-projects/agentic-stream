package domain

import (
	"fmt"
	"time"
)

// DefaultCapabilityTTL bounds an unspecified capability lifetime.
const DefaultCapabilityTTL = 15 * time.Minute

// PrepareScope defaults the validity window before a token identity is assigned.
func PrepareScope(scope Scope, now time.Time, maxTTL time.Duration) (Scope, error) {
	if scope.Traceparent == "" {
		return Scope{}, fmt.Errorf("traceparent is required")
	}
	scope.IssuedAt = timeOr(scope.IssuedAt, now)
	scope.NotBefore = timeOr(scope.NotBefore, now)
	scope.ExpiresAt = timeOr(scope.ExpiresAt, scope.IssuedAt.Add(maxTTL))
	return scope, nil
}

// CheckValidity verifies the token's time window with bounded clock skew.
func CheckValidity(scope Scope, now time.Time, maxTTL, skew time.Duration) error {
	if scope.ExpiresAt.Sub(scope.IssuedAt) > maxTTL {
		return fmt.Errorf("capability token lifetime exceeds maximum")
	}
	if now.Add(skew).Before(scope.NotBefore) || !now.Before(scope.ExpiresAt) || scope.IssuedAt.After(now.Add(skew)) {
		return fmt.Errorf("capability token is outside its validity window")
	}
	return nil
}

// CheckIssuable verifies the capability lifetime and authority names.
func CheckIssuable(scope Scope, now time.Time, maxTTL time.Duration) error {
	if scope.Issuer == "" || scope.Audience == "" {
		return fmt.Errorf("issuer and audience are required")
	}
	if scope.ExpiresAt.Sub(scope.IssuedAt) > maxTTL {
		return fmt.Errorf("capability token lifetime exceeds maximum")
	}
	if !scope.ExpiresAt.After(now) {
		return fmt.Errorf("capability token is already expired")
	}
	return nil
}

// ValidateScope requires an unambiguous authorization scope.
func ValidateScope(scope Scope) error {
	if !scope.identityComplete() {
		return fmt.Errorf("capability scope is incomplete")
	}
	if !scope.validityWindowValid() {
		return fmt.Errorf("capability validity window is invalid")
	}
	if !scope.evidenceRangeValid() {
		return fmt.Errorf("capability evidence range is invalid")
	}
	return nil
}

func (s Scope) identityComplete() bool {
	return s.KeyID != "" && s.EpisodeID != "" && s.AttemptID != "" && s.TenantID != "" && s.SituationID != "" &&
		s.EntityID != "" && s.Fence > 0 && len(s.Tools) > 0 && s.MaxRows > 0 && s.MaxBytes > 0
}

func (s Scope) validityWindowValid() bool {
	return !s.ExpiresAt.IsZero() && !s.NotBefore.IsZero() && !s.IssuedAt.IsZero() &&
		!s.IssuedAt.After(s.NotBefore) && s.NotBefore.Before(s.ExpiresAt)
}

func (s Scope) evidenceRangeValid() bool {
	return !s.Until.IsZero() && !s.From.IsZero() && !s.Until.Before(s.From) && s.Traceparent != "" &&
		s.SituationVersion > 0 && s.TokenID != "" && s.RuntimeEpoch != ""
}

func timeOr(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}
