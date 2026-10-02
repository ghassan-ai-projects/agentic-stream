// Package evidence implements the fail-closed boundary between an episode
// worker and bounded, read-only evidence tools.
package evidence

import (
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
	if scope.IssuedAt.IsZero() {
		scope.IssuedAt = now
	}
	if scope.NotBefore.IsZero() {
		scope.NotBefore = now
	}
	if scope.Traceparent == "" {
		return nil, fmt.Errorf("traceparent is required")
	}
	maxTTL := i.MaxTTL
	if maxTTL == 0 {
		maxTTL = defaultCapabilityTTL
	}
	if scope.ExpiresAt.IsZero() {
		// The token is an episode-attempt capability, not a durable credential.
		scope.ExpiresAt = scope.IssuedAt.Add(maxTTL)
	}
	id, err := tokenID(scope.TokenID)
	if err != nil {
		return nil, err
	}
	scope.TokenID = id
	if err := validateScope(scope); err != nil {
		return nil, err
	}
	key, ok := i.Keys[scope.KeyID]
	if !ok || len(key) < 32 {
		return nil, fmt.Errorf("signing key %q is unavailable or too short", scope.KeyID)
	}
	if scope.Issuer == "" {
		scope.Issuer = i.Issuer
	}
	if scope.Audience == "" {
		scope.Audience = i.Audience
	}
	if scope.Issuer == "" || scope.Audience == "" {
		return nil, fmt.Errorf("issuer and audience are required")
	}
	if scope.ExpiresAt.Sub(scope.IssuedAt) > maxTTL {
		return nil, fmt.Errorf("capability token lifetime exceeds maximum")
	}
	if !scope.ExpiresAt.After(now) {
		return nil, fmt.Errorf("capability token is already expired")
	}
	payload, err := json.Marshal(tokenPayload{
		Issuer: scope.Issuer, Audience: scope.Audience, TokenID: id, IssuedAt: scope.IssuedAt.UTC().Format(time.RFC3339Nano), EpisodeID: scope.EpisodeID,
		AttemptID: scope.AttemptID, Fence: scope.Fence, TenantID: scope.TenantID,
		SituationID: scope.SituationID, SituationVersion: scope.SituationVersion, EntityID: scope.EntityID, Tools: append([]string(nil), scope.Tools...),
		NotBefore: scope.NotBefore.UTC().Format(time.RFC3339Nano), ExpiresAt: scope.ExpiresAt.UTC().Format(time.RFC3339Nano),
		MaxRows: scope.MaxRows, MaxBytes: scope.MaxBytes,
		From: scope.From.UTC().Format(time.RFC3339Nano), Until: scope.Until.UTC().Format(time.RFC3339Nano), Traceparent: scope.Traceparent, Tracestate: scope.Tracestate, RuntimeEpoch: scope.RuntimeEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal capability payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(tokenVersion + "." + scope.KeyID + "." + encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return []byte(tokenVersion + "." + scope.KeyID + "." + encoded + "." + signature), nil
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
	parse := func(value, name string) (time.Time, error) {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid capability %s: %w", name, err)
		}
		return parsed.UTC(), nil
	}
	nbf, err := parse(p.NotBefore, "not-before")
	if err != nil {
		return Scope{}, err
	}
	exp, err := parse(p.ExpiresAt, "expiry")
	if err != nil {
		return Scope{}, err
	}
	from, err := parse(p.From, "from")
	if err != nil {
		return Scope{}, err
	}
	until, err := parse(p.Until, "until")
	if err != nil {
		return Scope{}, err
	}
	issued, err := parse(p.IssuedAt, "issued-at")
	if err != nil {
		return Scope{}, err
	}
	scope := Scope{KeyID: keyID, Issuer: p.Issuer, Audience: p.Audience, TokenID: p.TokenID, IssuedAt: issued, EpisodeID: p.EpisodeID, AttemptID: p.AttemptID, Fence: p.Fence, TenantID: p.TenantID, SituationID: p.SituationID, SituationVersion: p.SituationVersion, EntityID: p.EntityID, Tools: p.Tools, NotBefore: nbf, ExpiresAt: exp, MaxRows: p.MaxRows, MaxBytes: p.MaxBytes, From: from, Until: until, Traceparent: p.Traceparent, Tracestate: p.Tracestate, RuntimeEpoch: p.RuntimeEpoch}
	if err := validateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
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
