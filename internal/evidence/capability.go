// Package evidence implements bounded, capability-scoped evidence tools.
package evidence

import (
	"cmp"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
	"time"
)

// Scope is the complete authorization scope of one capability.
type Scope = domain.Scope

const defaultCapabilityTTL = domain.DefaultCapabilityTTL

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
	scope, err := prepareScope(scope, now, maxTTL)
	if err != nil {
		return Scope{}, err
	}
	scope.Issuer = cmp.Or(scope.Issuer, i.Issuer)
	scope.Audience = cmp.Or(scope.Audience, i.Audience)
	if err := domain.CheckIssuable(scope, now, maxTTL); err != nil {
		return Scope{}, err
	}
	return scope, nil
}
func prepareScope(scope Scope, now time.Time, maxTTL time.Duration) (Scope, error) {
	scope, err := domain.PrepareScope(scope, now, maxTTL)
	if err != nil {
		return Scope{}, err
	}
	if scope.TokenID, err = wire.TokenID(scope.TokenID); err != nil {
		return Scope{}, err
	}
	if err := domain.ValidateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// NewRuntimeEpoch generates an opaque owner identity.
func NewRuntimeEpoch() (string, error) { return wire.NewRuntimeEpoch() }
