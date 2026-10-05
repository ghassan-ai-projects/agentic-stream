package evidence

import (
	"cmp"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
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

// Verify authenticates the claims and their validity.
func (v *Verifier) Verify(token []byte) (Scope, error) {
	keyID, payloadBytes, err := wire.SignedPayload(token, v.Keys)
	if err != nil {
		return Scope{}, err
	}
	scope, err := verifiedScope(payloadBytes, keyID)
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
func (v *Verifier) checkValidity(scope Scope) error {
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	maxTTL := cmp.Or(v.MaxTTL, defaultCapabilityTTL)
	return domain.CheckValidity(scope, now, maxTTL, cmp.Or(v.ClockSkew, time.Second))
}

func verifiedScope(raw []byte, keyID string) (Scope, error) {
	scope, err := wire.DecodeScope(raw, keyID)
	if err != nil {
		return Scope{}, err
	}
	if err := domain.ValidateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}
