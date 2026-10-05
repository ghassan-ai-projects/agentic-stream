// Package app orders bounded evidence authorization, querying and durable lifecycle use cases.
package app

import (
	"cmp"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
	"time"
)

const defaultCapabilityTTL = domain.DefaultCapabilityTTL

// Issuer signs opaque capability tokens with an HMAC-SHA256 key ring.
type Issuer struct {
	Issuer   string
	Audience string
	KeyID    string
	Keys     map[string][]byte
	Now      func() time.Time
	MaxTTL   time.Duration
}

// Issue signs a short-lived attempt capability.
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
	return wire.SignToken(scope, key)
}
func (i *Issuer) completeScope(scope Scope, now time.Time) (Scope, error) {
	maxTTL := cmp.Or(i.MaxTTL, defaultCapabilityTTL)
	scope.KeyID = cmp.Or(scope.KeyID, i.KeyID)
	scope, err := prepareScope(scope, now, maxTTL)
	if err != nil {
		return Scope{}, err
	}
	return domain.CompleteScope(scope, domain.Authority{Issuer: i.Issuer, Audience: i.Audience, MaxTTL: maxTTL}, now)
}
func prepareScope(scope Scope, now time.Time, maxTTL time.Duration) (Scope, error) {
	scope, err := domain.PrepareScope(scope, now, maxTTL)
	if err != nil {
		return Scope{}, err
	}
	if scope.TokenID, err = wire.TokenID(scope.TokenID); err != nil {
		return Scope{}, err
	}
	return scope, nil
}
