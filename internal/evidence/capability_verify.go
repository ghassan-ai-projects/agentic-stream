package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Verifier validates token integrity, issuer/audience, time bounds, and the
// exact worker identity scope. It never returns a usable scope for a bad token.
type Verifier struct {
	Issuer    string
	Audience  string
	Keys      map[string][]byte
	Now       func() time.Time
	MaxTTL    time.Duration
	ClockSkew time.Duration
}

// Verify checks a token and returns its claims.
func (v *Verifier) Verify(token []byte) (Scope, error) {
	keyID, payloadBytes, err := v.signedPayload(token)
	if err != nil {
		return Scope{}, err
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return Scope{}, fmt.Errorf("decode capability claims: %w", err)
	}
	scope, err := payload.scope(keyID)
	if err != nil {
		return Scope{}, err
	}
	if scope.Issuer != v.Issuer || scope.Audience != v.Audience {
		return Scope{}, fmt.Errorf("capability issuer or audience mismatch")
	}
	if err := v.checkValidity(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// signedPayload checks the token format and HMAC signature in constant time
// and returns the signing key ID and the decoded claims bytes.
func (v *Verifier) signedPayload(token []byte) (string, []byte, error) {
	parts := strings.Split(string(token), ".")
	if len(parts) != 4 || parts[0] != tokenVersion || parts[1] == "" {
		return "", nil, fmt.Errorf("invalid capability token format")
	}
	key, ok := v.Keys[parts[1]]
	if !ok || len(key) < 32 {
		return "", nil, fmt.Errorf("unknown capability key")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.Join(parts[:3], ".")))
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(want) != sha256.Size || subtle.ConstantTimeCompare(mac.Sum(nil), want) != 1 {
		return "", nil, fmt.Errorf("invalid capability token signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", nil, fmt.Errorf("decode capability payload: %w", err)
	}
	return parts[1], payloadBytes, nil
}

// checkValidity bounds the token lifetime and requires now to fall inside its
// validity window, allowing for clock skew.
func (v *Verifier) checkValidity(scope Scope) error {
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	maxTTL := v.MaxTTL
	if maxTTL == 0 {
		maxTTL = defaultCapabilityTTL
	}
	if scope.ExpiresAt.Sub(scope.IssuedAt) > maxTTL {
		return fmt.Errorf("capability token lifetime exceeds maximum")
	}
	skew := v.ClockSkew
	if skew == 0 {
		skew = time.Second
	}
	if now.Add(skew).Before(scope.NotBefore) || !now.Before(scope.ExpiresAt) || scope.IssuedAt.After(now.Add(skew)) {
		return fmt.Errorf("capability token is outside its validity window")
	}
	return nil
}
