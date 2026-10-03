// Package evidence implements the fail-closed boundary between an episode
// worker and bounded, read-only evidence tools.
package evidence

import (
	"cmp"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const tokenVersion = "v1"

const defaultCapabilityTTL = 15 * time.Minute

// Scope is the complete authorization scope of one short-lived capability.
// It is intentionally bound to the durable worker identity and cannot grant
// access to effectors, credentials, filesystem, shell, or arbitrary network.
type Scope struct {
	KeyID            string
	Issuer           string
	Audience         string
	EpisodeID        string
	AttemptID        string
	Fence            int64
	TenantID         string
	SituationID      string
	SituationVersion int64
	EntityID         string
	TokenID          string
	IssuedAt         time.Time
	Tools            []string
	NotBefore        time.Time
	ExpiresAt        time.Time
	MaxRows          uint64
	MaxBytes         uint64
	From             time.Time
	Until            time.Time
	Traceparent      string
	Tracestate       string
	RuntimeEpoch     string
}

// Issuer signs opaque capability tokens with an HMAC-SHA256 key ring.
type Issuer struct {
	Issuer    string
	Audience  string
	KeyID     string
	Keys      map[string][]byte
	Now       func() time.Time
	MaxTTL    time.Duration
	ClockSkew time.Duration
}

// Issue creates a token. Invalid or incomplete scopes are rejected rather
// than producing a token that could be interpreted ambiguously.
func (i *Issuer) Issue(scope Scope) ([]byte, error) {
	now := time.Now().UTC()
	if i.Now != nil {
		now = i.Now().UTC()
	}
	scope, err := i.completeScope(scope, now)
	if err != nil {
		return nil, err
	}
	key, ok := i.Keys[scope.KeyID]
	if !ok || len(key) < 32 {
		return nil, fmt.Errorf("signing key %q is unavailable or too short", scope.KeyID)
	}
	return signToken(scope, key)
}

// completeScope fills the scope's defaults (issue time, validity window,
// token ID, issuer, audience) and requires a complete scope whose lifetime is
// within the maximum and not yet over.
func (i *Issuer) completeScope(scope Scope, now time.Time) (Scope, error) {
	maxTTL := cmp.Or(i.MaxTTL, defaultCapabilityTTL)
	scope, err := prepareScope(scope, now, maxTTL)
	if err != nil {
		return Scope{}, err
	}
	scope.Issuer = cmp.Or(scope.Issuer, i.Issuer)
	scope.Audience = cmp.Or(scope.Audience, i.Audience)
	if err := checkIssuable(scope, now, maxTTL); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// prepareScope requires a trace, defaults the validity window (the token is
// an episode-attempt capability, not a durable credential), assigns a token
// ID and validates the scope.
func prepareScope(scope Scope, now time.Time, maxTTL time.Duration) (Scope, error) {
	if scope.Traceparent == "" {
		return Scope{}, fmt.Errorf("traceparent is required")
	}
	scope.IssuedAt = timeOr(scope.IssuedAt, now)
	scope.NotBefore = timeOr(scope.NotBefore, now)
	scope.ExpiresAt = timeOr(scope.ExpiresAt, scope.IssuedAt.Add(maxTTL))
	var err error
	if scope.TokenID, err = tokenID(scope.TokenID); err != nil {
		return Scope{}, err
	}
	if err := validateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// checkIssuable requires an issuer and audience and a lifetime that is
// within the maximum and not already over.
func checkIssuable(scope Scope, now time.Time, maxTTL time.Duration) error {
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

// signToken encodes the scope's claims and signs version, key ID, and claims
// with HMAC-SHA256.
func signToken(scope Scope, key []byte) ([]byte, error) {
	payload, err := json.Marshal(newTokenPayload(scope))
	if err != nil {
		return nil, fmt.Errorf("marshal capability payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(tokenVersion + "." + scope.KeyID + "." + encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return []byte(tokenVersion + "." + scope.KeyID + "." + encoded + "." + signature), nil
}

func newTokenPayload(scope Scope) tokenPayload {
	return tokenPayload{
		Issuer: scope.Issuer, Audience: scope.Audience, TokenID: scope.TokenID, IssuedAt: scope.IssuedAt.UTC().Format(time.RFC3339Nano), EpisodeID: scope.EpisodeID,
		AttemptID: scope.AttemptID, Fence: scope.Fence, TenantID: scope.TenantID,
		SituationID: scope.SituationID, SituationVersion: scope.SituationVersion, EntityID: scope.EntityID, Tools: append([]string(nil), scope.Tools...),
		NotBefore: scope.NotBefore.UTC().Format(time.RFC3339Nano), ExpiresAt: scope.ExpiresAt.UTC().Format(time.RFC3339Nano),
		MaxRows: scope.MaxRows, MaxBytes: scope.MaxBytes,
		From: scope.From.UTC().Format(time.RFC3339Nano), Until: scope.Until.UTC().Format(time.RFC3339Nano), Traceparent: scope.Traceparent, Tracestate: scope.Tracestate, RuntimeEpoch: scope.RuntimeEpoch,
	}
}

func validateScope(scope Scope) error {
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

type tokenPayload struct {
	Issuer           string   `json:"iss"`
	Audience         string   `json:"aud"`
	EpisodeID        string   `json:"episode_id"`
	AttemptID        string   `json:"attempt_id"`
	Fence            int64    `json:"fence"`
	TenantID         string   `json:"tenant_id"`
	SituationID      string   `json:"situation_id"`
	SituationVersion int64    `json:"situation_version"`
	EntityID         string   `json:"entity_id"`
	TokenID          string   `json:"jti"`
	IssuedAt         string   `json:"iat"`
	Tools            []string `json:"tools"`
	NotBefore        string   `json:"nbf"`
	ExpiresAt        string   `json:"exp"`
	MaxRows          uint64   `json:"max_rows"`
	MaxBytes         uint64   `json:"max_bytes"`
	From             string   `json:"from"`
	Until            string   `json:"until"`
	Traceparent      string   `json:"traceparent"`
	Tracestate       string   `json:"tracestate,omitempty"`
	RuntimeEpoch     string   `json:"runtime_epoch"`
}

func (p tokenPayload) scope(keyID string) (Scope, error) {
	scope := Scope{KeyID: keyID, Issuer: p.Issuer, Audience: p.Audience, TokenID: p.TokenID, EpisodeID: p.EpisodeID, AttemptID: p.AttemptID, Fence: p.Fence, TenantID: p.TenantID, SituationID: p.SituationID, SituationVersion: p.SituationVersion, EntityID: p.EntityID, Tools: p.Tools, MaxRows: p.MaxRows, MaxBytes: p.MaxBytes, Traceparent: p.Traceparent, Tracestate: p.Tracestate, RuntimeEpoch: p.RuntimeEpoch}
	if err := p.decodeTimes(&scope); err != nil {
		return Scope{}, err
	}
	if err := validateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

func (p tokenPayload) decodeTimes(scope *Scope) error {
	fields := []struct {
		value, name string
		target      *time.Time
	}{
		{p.NotBefore, "not-before", &scope.NotBefore}, {p.ExpiresAt, "expiry", &scope.ExpiresAt},
		{p.From, "from", &scope.From}, {p.Until, "until", &scope.Until}, {p.IssuedAt, "issued-at", &scope.IssuedAt},
	}
	for _, field := range fields {
		parsed, err := time.Parse(time.RFC3339Nano, field.value)
		if err != nil {
			return fmt.Errorf("invalid capability %s: %w", field.name, err)
		}
		*field.target = parsed.UTC()
	}
	return nil
}

func tokenID(existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate capability token id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// NewRuntimeEpoch creates an opaque process-owner identity. It is intended to
// be generated once at startup and shared by the ledger, token issuer, and
// EvidenceTools server; it must never be persisted as a secret.
func NewRuntimeEpoch() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate runtime epoch: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// timeOr returns value unless it is the zero time.
func timeOr(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}
