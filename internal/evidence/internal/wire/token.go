// Package wire encodes evidence tokens, arguments and result records without authorization decisions.
package wire

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

const tokenVersion = "v1"

// SignToken preserves the v1 signed claim encoding.
func SignToken(scope domain.Scope, key []byte) ([]byte, error) {
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

func newTokenPayload(scope domain.Scope) tokenPayload {
	return tokenPayload{
		Issuer: scope.Issuer, Audience: scope.Audience, TokenID: scope.TokenID, IssuedAt: scope.IssuedAt.UTC().Format(time.RFC3339Nano), EpisodeID: scope.EpisodeID,
		AttemptID: scope.AttemptID, Fence: scope.Fence, TenantID: scope.TenantID,
		SituationID: scope.SituationID, SituationVersion: scope.SituationVersion, EntityID: scope.EntityID, Tools: append([]string(nil), scope.Tools...),
		NotBefore: scope.NotBefore.UTC().Format(time.RFC3339Nano), ExpiresAt: scope.ExpiresAt.UTC().Format(time.RFC3339Nano),
		MaxRows: scope.MaxRows, MaxBytes: scope.MaxBytes,
		From: scope.From.UTC().Format(time.RFC3339Nano), Until: scope.Until.UTC().Format(time.RFC3339Nano), Traceparent: scope.Traceparent, Tracestate: scope.Tracestate, RuntimeEpoch: scope.RuntimeEpoch,
	}
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

func (p tokenPayload) scope(keyID string) (domain.Scope, error) {
	scope := domain.Scope{KeyID: keyID, Issuer: p.Issuer, Audience: p.Audience, TokenID: p.TokenID, EpisodeID: p.EpisodeID, AttemptID: p.AttemptID, Fence: p.Fence, TenantID: p.TenantID, SituationID: p.SituationID, SituationVersion: p.SituationVersion, EntityID: p.EntityID, Tools: p.Tools, MaxRows: p.MaxRows, MaxBytes: p.MaxBytes, Traceparent: p.Traceparent, Tracestate: p.Tracestate, RuntimeEpoch: p.RuntimeEpoch}
	if err := p.decodeTimes(&scope); err != nil {
		return domain.Scope{}, err
	}
	return scope, nil
}

func (p tokenPayload) decodeTimes(scope *domain.Scope) error {
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

// SignedPayload checks framing and HMAC integrity before returning claim bytes.
func SignedPayload(token []byte, keys map[string][]byte) (string, []byte, error) {
	parts := strings.Split(string(token), ".")
	if len(parts) != 4 || parts[0] != tokenVersion || parts[1] == "" {
		return "", nil, fmt.Errorf("invalid capability token format")
	}
	if err := checkSignature(parts, keys); err != nil {
		return "", nil, err
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", nil, fmt.Errorf("decode capability payload: %w", err)
	}
	return parts[1], payloadBytes, nil
}

func checkSignature(parts []string, keys map[string][]byte) error {
	key, ok := keys[parts[1]]
	if !ok || len(key) < 32 {
		return fmt.Errorf("unknown capability key")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.Join(parts[:3], ".")))
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(want) != sha256.Size || subtle.ConstantTimeCompare(mac.Sum(nil), want) != 1 {
		return fmt.Errorf("invalid capability token signature")
	}
	return nil
}

// DecodeScope decodes claims; application code validates their scope.
func DecodeScope(raw []byte, keyID string) (domain.Scope, error) {
	var payload tokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.Scope{}, fmt.Errorf("decode capability claims: %w", err)
	}
	return payload.scope(keyID)
}
